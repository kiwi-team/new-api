import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import deploy_mixrouter as deploy


class Clock:
    def __init__(self):
        self.now = 0

    def monotonic(self):
        return self.now

    def sleep(self, seconds):
        self.now += seconds


class DeploymentHost(deploy.Deployment):
    """Use real files, replacing Linux process, Nginx and systemd operations."""

    def __init__(self, arguments, instances):
        super().__init__(arguments, instances)
        self.commands = []
        self.generation = 1
        self.current_workers = {(10, "100"): False}
        self.fail_new_port = None
        self.fail_rollback = False
        self.hold_workers = False
        self.reject_next_config = False
        self.fail_enable_reload = False
        self.reload_failed = False
        self.new_probe_pids = []

    def run(self, *command, timeout=30):
        self.commands.append(command)
        if command[1] == "-T":
            return self.config.read_text()
        if command[1] == "-t" and self.reject_next_config:
            self.reject_next_config = False
            raise RuntimeError("invalid Nginx configuration")
        if command[1] == "-s":
            self.generation += 1
            workers = {worker: True for worker in self.current_workers} if self.hold_workers else {}
            workers[(10 * self.generation, str(100 * self.generation))] = False
            self.current_workers = workers
            if (self.fail_enable_reload and not self.reload_failed
                    and self.instances[0][1].read_bytes() == self.binary.read_bytes()
                    and "3000 max_fails=1 fail_timeout=5s down;" not in self.config.read_text()):
                self.reload_failed = True
                raise RuntimeError("reload command failed after signaling Nginx")
        if command[:2] == ("systemctl", "restart"):
            # A restart may only occur after old Nginx workers have finished.
            if any(self.current_workers.values()):
                raise AssertionError("restarted while old Nginx workers still serve requests")
            port = next(port for port, _, service in self.instances if service == command[2])
            if "{} max_fails=1 fail_timeout=5s down;".format(port) not in self.config.read_text():
                raise AssertionError("restarted an instance still enabled in Nginx")
        return ""

    def workers(self):
        return 42, dict(self.current_workers)

    def probe(self, instance):
        port, binary, _ = instance
        is_new = binary.read_bytes() == self.binary.read_bytes()
        if port == self.fail_new_port and (is_new or self.fail_rollback):
            raise RuntimeError("instance is unhealthy")
        pid = self.new_probe_pids.pop(0) if is_new and self.new_probe_pids else port
        return pid, "new" if is_new else "old"


class MixrouterDeploymentTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.instances = (
            (3000, self.root / "one-api" / "oneapi", "oneapi.service"),
            (3001, self.root / "one-api2" / "oneapi", "oneapi2.service"),
        )
        for port, binary, _ in self.instances:
            binary.parent.mkdir()
            binary.write_bytes("old-{}".format(port).encode())
            binary.chmod(0o750)
        self.upload = self.root / "oneapi.upload"
        header = bytearray(20)
        header[:6] = b"\x7fELF\x02\x01"
        header[18:20] = (62).to_bytes(2, "little")
        self.upload.write_bytes(bytes(header) + b"new binary")
        self.pid_file = self.root / "nginx.pid"
        self.pid_file.write_text("42")
        self.config = self.root / "nginx.conf"
        self.original_config = """pid %s;
events {}
http {
    upstream unrelated {
        server 127.0.0.1:3000;
    }
    upstream mixrouter_backend {
        zone mixrouter_backend 64k;
        least_conn;
        server 127.0.0.1:3000 max_fails=1 fail_timeout=5s; # primary
        server 127.0.0.1:3001 max_fails=1 fail_timeout=5s; # secondary
        keepalive 64;
    }
    server {
        location / { proxy_pass http://mixrouter_backend; }
        location /v1/realtime { proxy_pass http://mixrouter_backend; }
    }
}
""" % self.pid_file
        # Match the multiline locations in the supplied production configuration.
        self.original_config = self.original_config.replace(
            "{ proxy_pass http://mixrouter_backend; }",
            "{\n            proxy_pass http://mixrouter_backend;\n        }",
        )
        self.config.write_text(self.original_config)
        self.config.chmod(0o640)
        arguments = argparse.Namespace(
            binary=str(self.upload), sha256=deploy.file_digest(self.upload),
            nginx_config=str(self.config), nginx_pid_file=str(self.pid_file),
            drain_timeout=5, health_timeout=5,
        )
        self.host = DeploymentHost(arguments, self.instances)
        self.clock = Clock()
        for patcher in (
            mock.patch.object(deploy, "log"),
            mock.patch.object(deploy.time, "monotonic", self.clock.monotonic),
            mock.patch.object(deploy.time, "sleep", self.clock.sleep),
        ):
            patcher.start()
            self.addCleanup(patcher.stop)

    def restarts(self):
        return [command[2] for command in self.host.commands if command[:2] == ("systemctl", "restart")]

    def test_success_deploys_3000_then_3001_and_retains_backups_and_permissions(self):
        self.host.deploy()
        self.assertEqual(self.restarts(), ["oneapi.service", "oneapi2.service"])
        for port, binary, _ in self.instances:
            self.assertEqual(binary.read_bytes(), self.upload.read_bytes())
            self.assertEqual(binary.stat().st_mode & 0o777, 0o750)
            backups = list(binary.parent.glob("oneapi.bak.*"))
            self.assertEqual(len(backups), 1)
            self.assertEqual(backups[0].read_bytes(), "old-{}".format(port).encode())
        self.assertEqual(self.config.read_text(), self.original_config)
        self.assertEqual(self.config.stat().st_mode & 0o777, 0o640)

    def test_drain_timeout_restores_traffic_without_replacing_or_restarting(self):
        self.host.hold_workers = True
        with self.assertRaisesRegex(RuntimeError, "已恢复"):
            self.host.deploy()
        self.assertEqual(self.restarts(), [])
        self.assertEqual(self.instances[0][1].read_bytes(), b"old-3000")
        self.assertEqual(self.instances[1][1].read_bytes(), b"old-3001")
        self.assertEqual(self.config.read_text(), self.original_config)

    def test_invalid_nginx_candidate_restores_config_before_any_restart(self):
        self.host.reject_next_config = True
        with self.assertRaisesRegex(RuntimeError, "已恢复"):
            self.host.deploy()
        self.assertEqual(self.restarts(), [])
        self.assertEqual(self.config.read_text(), self.original_config)

    def test_first_instance_failure_rolls_back_and_stops_before_second_instance(self):
        self.host.fail_new_port = 3000
        with self.assertRaisesRegex(RuntimeError, "已恢复"):
            self.host.deploy()
        self.assertEqual(self.restarts(), ["oneapi.service", "oneapi.service"])
        self.assertEqual(self.instances[0][1].read_bytes(), b"old-3000")
        self.assertEqual(self.instances[1][1].read_bytes(), b"old-3001")
        self.assertEqual(self.config.read_text(), self.original_config)

    def test_second_instance_failure_keeps_updated_first_instance_and_recovers_second(self):
        self.host.fail_new_port = 3001
        with self.assertRaisesRegex(RuntimeError, "已恢复"):
            self.host.deploy()
        self.assertEqual(self.restarts(), ["oneapi.service", "oneapi2.service", "oneapi2.service"])
        self.assertEqual(self.instances[0][1].read_bytes(), self.upload.read_bytes())
        self.assertEqual(self.instances[1][1].read_bytes(), b"old-3001")
        self.assertEqual(self.config.read_text(), self.original_config)

    def test_failed_rollback_keeps_unhealthy_instance_down(self):
        self.host.fail_new_port = 3000
        self.host.fail_rollback = True
        with self.assertRaisesRegex(RuntimeError, "需要人工恢复"):
            self.host.deploy()
        self.assertEqual(self.restarts(), ["oneapi.service", "oneapi.service"])
        self.assertIn("3000 max_fails=1 fail_timeout=5s down;", self.config.read_text())
        self.assertNotIn("3001 max_fails=1 fail_timeout=5s down;", self.config.read_text())

    def test_enable_reload_failure_drains_again_before_rolling_back(self):
        self.host.fail_enable_reload = True
        with self.assertRaisesRegex(RuntimeError, "已恢复"):
            self.host.deploy()
        self.assertTrue(self.host.reload_failed)
        self.assertEqual(self.restarts(), ["oneapi.service", "oneapi.service"])
        self.assertEqual(self.instances[0][1].read_bytes(), b"old-3000")
        self.assertEqual(self.config.read_text(), self.original_config)

    def test_other_configuration_and_concurrent_edits_are_preserved(self):
        self.host.set_backend(3000, down=True)
        expected = self.original_config.replace(
            "3000 max_fails=1 fail_timeout=5s;", "3000 max_fails=1 fail_timeout=5s down;",
        )
        self.assertEqual(self.config.read_text(), expected)
        manually_edited = expected + "# operator edit\n"
        self.config.write_text(manually_edited)
        with self.assertRaisesRegex(RuntimeError, "其他操作修改"):
            self.host.set_backend(3000, down=False)
        self.assertEqual(self.config.read_text(), manually_edited)

    def test_atomic_install_keeps_open_old_executable_unchanged(self):
        binary = self.instances[0][1]
        with binary.open("rb") as old_executable:
            deploy.atomic_install(self.upload, binary)
            self.assertEqual(old_executable.read(), b"old-3000")
        self.assertEqual(binary.read_bytes(), self.upload.read_bytes())

    def test_preflight_rejects_direct_routing_and_bad_upload_without_mutation(self):
        with mock.patch.object(deploy.platform, "system", return_value="Linux"), \
                mock.patch.object(deploy.platform, "machine", return_value="x86_64"):
            self.host.preflight()
            self.assertEqual(self.restarts(), [])
            self.config.write_text(self.original_config + "proxy_pass http://127.0.0.1:3000;\n")
            with self.assertRaisesRegex(RuntimeError, "直连"):
                self.host.preflight()
            self.upload.write_bytes(b"corrupted upload")
            with self.assertRaisesRegex(RuntimeError, "SHA-256"):
                self.host.preflight()
        self.assertEqual(self.restarts(), [])

    def test_check_mode_performs_preflight_without_deploying(self):
        arguments = ["deploy_mixrouter.py", "--binary", str(self.upload), "--sha256", self.host.digest, "--check"]
        with mock.patch.object(sys, "argv", arguments), \
                mock.patch.object(deploy.os, "geteuid", return_value=0), \
                mock.patch("builtins.open", mock.mock_open()), \
                mock.patch.object(deploy.fcntl, "flock"), \
                mock.patch.object(deploy, "Deployment") as deployment_class:
            self.assertEqual(deploy.main(), 0)
        deployment_class.return_value.preflight.assert_called_once_with()
        deployment_class.return_value.deploy.assert_not_called()
        self.assertEqual(self.config.read_text(), self.original_config)

    def test_preflight_rejects_forced_worker_shutdown_but_accepts_disabled_timeout(self):
        with mock.patch.object(deploy.platform, "system", return_value="Linux"), \
                mock.patch.object(deploy.platform, "machine", return_value="x86_64"):
            self.config.write_text("worker_shutdown_timeout    0;\n" + self.original_config)
            self.host.preflight()
            self.config.write_text("worker_shutdown_timeout 30s;\n" + self.original_config)
            with self.assertRaisesRegex(RuntimeError, "强制关闭旧连接"):
                self.host.preflight()
        self.assertEqual(self.restarts(), [])

    def test_probe_rejects_a_port_owned_by_another_process(self):
        with mock.patch.object(self.host, "run", side_effect=["", "1234", "users:((oneapi,pid=9999,fd=6))"]), \
                mock.patch.object(os.path, "samefile", return_value=True):
            with self.assertRaisesRegex(RuntimeError, "未由 oneapi.service"):
                deploy.Deployment.probe(self.host, self.instances[0])

    def test_readiness_requires_three_successes_from_the_same_process(self):
        deploy.atomic_install(self.upload, self.instances[0][1])
        self.host.new_probe_pids = [1, 2, 3]
        with self.assertRaisesRegex(RuntimeError, "未就绪"):
            self.host.wait_healthy(self.instances[0])

    def test_shell_entry_handles_bash_32_check_mode_and_quoted_arguments(self):
        fake_bin = self.root / "bin"
        fake_bin.mkdir()
        captured = self.root / "remote-arguments.json"
        ssh = fake_bin / "ssh"
        ssh.write_text("#!{}\n".format(sys.executable) + """import json, os, shlex, sys
from pathlib import Path
command = sys.argv[-1]
if 'mktemp -d' in command:
    print('/var/tmp/mixrouter-deploy.TESTING')
elif command.startswith('python3 '):
    Path(os.environ['CAPTURED_ARGUMENTS']).write_text(json.dumps(shlex.split(command)))
else:
    raise SystemExit('unexpected SSH command: ' + command)
""")
        ssh.chmod(0o755)
        scp = fake_bin / "scp"
        scp.write_text("#!/bin/sh\nexit 0\n")
        scp.chmod(0o755)
        binary = self.root / "new oneapi 'quoted' $(touch SHOULD_NOT_RUN)"
        binary.write_bytes(self.upload.read_bytes())
        nginx_config = "/etc/nginx/config 'quoted' $(touch SHOULD_NOT_RUN).conf"
        environment = dict(os.environ, PATH=str(fake_bin) + os.pathsep + os.environ["PATH"],
                           CAPTURED_ARGUMENTS=str(captured), NGINX_CONFIG=nginx_config)
        script = Path(__file__).resolve().parents[1] / "deploy-mixrouter-new.sh"
        # On macOS /bin/bash is 3.2; use the system Bash on other platforms too.
        for flags in ([], ["--check"]):
            with self.subTest(flags=flags):
                result = subprocess.run(["/bin/bash", str(script)] + flags + [str(binary)],
                                        cwd=str(self.root), env=environment,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                        universal_newlines=True, timeout=20)
                self.assertEqual(result.returncode, 0, result.stderr)
                arguments = json.loads(captured.read_text())
                self.assertEqual(arguments[arguments.index("--nginx-config") + 1], nginx_config)
                self.assertEqual("--check" in arguments, bool(flags))
                self.assertFalse((self.root / "SHOULD_NOT_RUN").exists())


if __name__ == "__main__":
    unittest.main()
