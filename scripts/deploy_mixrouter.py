#!/usr/bin/env python3
"""Mixrouter rolling deployment runtime, uploaded by deploy-mixrouter-new.sh."""

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import urllib.request


INSTANCES = (
    (3000, Path("/data/www/one-api/oneapi"), "oneapi.service"),
    (3001, Path("/data/www/one-api2/oneapi"), "oneapi2.service"),
)


def log(message):
    try:
        print(time.strftime("[%Y-%m-%d %H:%M:%S] ") + message, flush=True)
    except BrokenPipeError:
        # SSH 已断开时仍需执行当前实例的恢复流程。
        pass


def file_digest(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def atomic_install(source, destination):
    """Replace a file on its filesystem, retaining its owner, mode and xattrs."""
    metadata = destination.stat()
    descriptor, name = tempfile.mkstemp(prefix=".mixrouter-", dir=str(destination.parent))
    os.close(descriptor)
    temporary = Path(name)
    try:
        shutil.copyfile(str(source), str(temporary))
        os.chown(str(temporary), metadata.st_uid, metadata.st_gid)
        shutil.copystat(str(destination), str(temporary))
        os.replace(str(temporary), str(destination))
    finally:
        if temporary.exists():
            temporary.unlink()


def upstream_members(config):
    """Locate the two inline servers in the upstream from nginx.mixrouter.conf."""
    blocks = list(re.finditer(
        r"(?ms)^[ \t]*upstream[ \t]+mixrouter_backend[ \t]*\{[^\n]*\n"
        r"(?P<body>.*?)^[ \t]*\}", config,
    ))
    if len(blocks) != 1:
        raise RuntimeError("Nginx 配置必须包含一个 mixrouter_backend upstream；请先安装 nginx.mixrouter.conf")
    block = blocks[0]
    body = block.group("body")
    members = list(re.finditer(
        r"(?m)^(?P<prefix>[ \t]*server[ \t]+127\.0\.0\.1:)"
        r"(?P<port>3000|3001)(?P<parameters>[^;\n]*);(?P<comment>[ \t]*(?:#[^\n]*)?)$",
        body,
    ))
    directives = re.findall(r"(?m)^[ \t]*(?:server|include)\b", body)
    if len(directives) != 2 or sorted(member.group("port") for member in members) != ["3000", "3001"]:
        raise RuntimeError("mixrouter_backend 必须直接列出 127.0.0.1:3000 和 127.0.0.1:3001，各占一行")
    return block, members


class Deployment:
    def __init__(self, arguments, instances=INSTANCES):
        self.binary = Path(arguments.binary)
        self.digest = arguments.sha256
        self.config = Path(arguments.nginx_config).resolve()
        self.pid_file = Path(arguments.nginx_pid_file)
        self.drain_timeout = arguments.drain_timeout
        self.health_timeout = arguments.health_timeout
        self.instances = instances
        self.work_dir = self.binary.parent
        self.nginx = shutil.which("nginx") or "/usr/sbin/nginx"
        self.config_text = self.config.read_text(encoding="utf-8")
        self.release_id = time.strftime("%Y%m%d-%H%M%S") + "-" + str(os.getpid())

    def run(self, *command, timeout=30):
        result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                universal_newlines=True, timeout=timeout)
        if result.returncode:
            raise RuntimeError("命令失败：{}\n{}".format(" ".join(command), result.stderr.strip()))
        return result.stdout.strip()

    def probe(self, instance):
        port, binary, service = instance
        self.run("systemctl", "is-active", "--quiet", service)
        pid = int(self.run("systemctl", "show", "--property=MainPID", "--value", service))
        if pid <= 0 or not os.path.samefile(str(binary), "/proc/{}/exe".format(pid)):
            raise RuntimeError("{} 没有运行当前路径的二进制：{}".format(service, binary))
        listeners = self.run("ss", "-H", "-ltnp", "sport = :{}".format(port))
        if not re.search(r"\bpid={},".format(pid), listeners):
            raise RuntimeError("端口 {} 未由 {} 的 MainPID={} 监听".format(port, service, pid))
        # 避免服务器 HTTP_PROXY 等环境变量影响本机健康检查。
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        request = urllib.request.Request("http://127.0.0.1:{}/api/status".format(port))
        with opener.open(request, timeout=3) as response:
            status = json.load(response)
        if not isinstance(status, dict) or status.get("success") is not True:
            raise RuntimeError("端口 {} 的 /api/status 检查未通过".format(port))
        return pid, status.get("data", {}).get("version", "unknown")

    def wait_healthy(self, instance):
        deadline = time.monotonic() + self.health_timeout
        previous_pid, successes, last_error = None, 0, "尚未响应"
        while time.monotonic() < deadline:
            try:
                pid, version = self.probe(instance)
                successes = successes + 1 if pid == previous_pid else 1
                previous_pid = pid
                if successes >= 3:
                    log("端口 {} 连续三次检查通过，版本 {}".format(instance[0], version))
                    return
            except (OSError, ValueError, RuntimeError) as error:
                previous_pid, successes, last_error = None, 0, str(error)
            time.sleep(min(2, max(0, deadline - time.monotonic())))
        raise RuntimeError("端口 {} 在 {} 秒内未就绪：{}".format(instance[0], self.health_timeout, last_error))

    def workers(self):
        master = int(self.pid_file.read_text().strip())
        workers = {}
        for directory in Path("/proc").iterdir():
            if not directory.name.isdigit():
                continue
            try:
                fields = (directory / "stat").read_text().rsplit(")", 1)[1].split()
                command = (directory / "cmdline").read_bytes().replace(b"\0", b" ")
            except (FileNotFoundError, ProcessLookupError):
                continue
            if int(fields[1]) == master and command.startswith(b"nginx: worker process"):
                workers[(int(directory.name), fields[19])] = b"is shutting down" in command
        return master, workers

    def reload(self, drain=False, peer=None):
        # Nginx keeps pre-reload workers serving existing clients:
        # https://nginx.org/en/docs/control.html
        self.run(self.nginx, "-t", "-c", str(self.config))
        master, old_workers = self.workers()
        if not old_workers:
            raise RuntimeError("找不到 Nginx worker，停止发布")
        self.run(self.nginx, "-s", "reload", "-c", str(self.config))
        deadline = time.monotonic() + 30
        while True:
            current_master, current = self.workers()
            if current_master != master:
                raise RuntimeError("Nginx master 在 reload 期间发生变化，停止发布")
            new_workers = set(current) - set(old_workers)
            if (any(not current[worker] for worker in new_workers)
                    and all(current.get(worker, True) for worker in old_workers)):
                break
            if time.monotonic() >= deadline:
                raise RuntimeError("Nginx reload 后未确认新 worker 已接管，停止发布")
            time.sleep(0.2)
        if not drain:
            return
        deadline = time.monotonic() + self.drain_timeout
        next_report = next_peer_check = 0
        while True:
            current_master, current = self.workers()
            if current_master != master:
                raise RuntimeError("等待旧请求期间 Nginx master 发生变化，停止发布")
            remaining = set(old_workers) & set(current)
            if not remaining:
                log("旧 Nginx worker 已全部退出")
                return
            now = time.monotonic()
            if now >= deadline:
                raise RuntimeError("等待旧连接超过 {} 秒，中止发布，保留正在运行的实例".format(self.drain_timeout))
            if peer is not None and now >= next_peer_check:
                self.probe(peer)
                next_peer_check = now + 10
            if now >= next_report:
                log("等待 {} 个旧 worker 结束请求，最多还等待 {} 秒".format(len(remaining), int(deadline - now)))
                next_report = now + 15
            time.sleep(min(2, max(0, deadline - time.monotonic())))

    def write_config(self, content):
        if self.config.read_text(encoding="utf-8") != self.config_text:
            raise RuntimeError("Nginx 配置被其他操作修改，停止覆盖；请检查现场配置")
        pending = self.work_dir / "nginx.pending"
        pending.write_text(content, encoding="utf-8")
        atomic_install(pending, self.config)
        self.config_text = content

    def set_backend(self, port, down):
        block, members = upstream_members(self.config_text)
        member = next(item for item in members if item.group("port") == str(port))
        parameters = [value for value in member.group("parameters").split() if value != "down"]
        if down:
            parameters.append("down")
        line = member.group("prefix") + str(port)
        if parameters:
            line += " " + " ".join(parameters)
        line += ";" + member.group("comment")
        start = block.start("body") + member.start()
        end = block.start("body") + member.end()
        self.write_config(self.config_text[:start] + line + self.config_text[end:])

    def preflight(self):
        if platform.system() != "Linux":
            raise RuntimeError("远端发布程序需要 Linux")
        if file_digest(self.binary) != self.digest:
            raise RuntimeError("上传的二进制 SHA-256 与本地不一致")
        with self.binary.open("rb") as source:
            header = source.read(20)
        machine = {"x86_64": 62, "aarch64": 183}.get(platform.machine())
        if len(header) < 20 or header[:5] != b"\x7fELF\x02" or header[5] not in (1, 2):
            raise RuntimeError("请上传 Linux 64 位 ELF 二进制")
        byte_order = "little" if header[5] == 1 else "big"
        if machine is None or int.from_bytes(header[18:20], byte_order) != machine:
            raise RuntimeError("二进制架构与服务器 {} 不匹配".format(platform.machine()))
        _, members = upstream_members(self.config_text)
        if any("down" in member.group("parameters").split() for member in members):
            raise RuntimeError("发布开始前必须确认 3000、3001 都已恢复为正常接流量状态")
        loaded = self.run(self.nginx, "-T", "-c", str(self.config))
        if re.search(r"(?m)^[ \t]*proxy_pass[ \t]+http://(?:127\.0\.0\.1|localhost|\[::1\]):300[01](?:[/;\s])", loaded):
            raise RuntimeError("Nginx 仍有直连 3000/3001 的 proxy_pass，请全部接入 mixrouter_backend")
        if len(re.findall(r"(?m)^[ \t]*proxy_pass[ \t]+http://mixrouter_backend\s*;", loaded)) < 2:
            raise RuntimeError("普通接口和 Realtime 必须都通过 mixrouter_backend 转发")
        shutdown_timeouts = re.findall(r"(?m)^[ \t]*worker_shutdown_timeout[ \t]+([^;]+);", loaded)
        if any(not re.fullmatch(r"0(?:ms|s|m|h|d|w)?", value.strip()) for value in shutdown_timeouts):
            raise RuntimeError("请移除 Nginx worker_shutdown_timeout，避免等待期间强制关闭旧连接")
        pid_paths = re.findall(r"(?m)^[ \t]*pid[ \t]+([^;\s]+)\s*;", loaded)
        if len(pid_paths) != 1 or Path(pid_paths[0]).resolve() != self.pid_file.resolve():
            raise RuntimeError("NGINX_PID_FILE 与 Nginx 配置中的 pid 路径不一致")
        if not self.workers()[1]:
            raise RuntimeError("找不到运行中的 Nginx worker")
        for instance in self.instances:
            binary = instance[1]
            if binary.is_symlink() or not binary.is_file() or not binary.stat().st_mode & 0o111:
                raise RuntimeError("服务程序必须是可执行的普通文件：{}".format(binary))
            self.probe(instance)
        log("预检查通过：两个服务、端口归属、二进制及 Nginx 配置正常")

    def deploy_instance(self, instance, peer):
        port, binary, service = instance
        original_config = self.config_text
        backup = binary.with_name(binary.name + ".bak." + self.release_id)
        binary_changed = traffic_enabled = False
        try:
            self.probe(peer)
            log("摘除端口 {}，由端口 {} 接收新请求".format(port, peer[0]))
            self.set_backend(port, down=True)
            self.reload(drain=True, peer=peer)
            self.probe(peer)

            shutil.copy2(str(binary), str(backup))
            metadata = binary.stat()
            os.chown(str(backup), metadata.st_uid, metadata.st_gid)
            log("备份 {}，更新 {}".format(backup, service))
            binary_changed = True
            atomic_install(self.binary, binary)
            if file_digest(binary) != self.digest:
                raise RuntimeError("安装后的二进制 SHA-256 不匹配")
            self.run("systemctl", "restart", service, timeout=None)
            self.wait_healthy(instance)

            traffic_enabled = True
            self.set_backend(port, down=False)
            self.reload()
            log("端口 {} 发布完成，已恢复流量".format(port))
        except BaseException as error:
            log("端口 {} 发布失败：{}".format(port, error))
            try:
                if binary_changed:
                    if traffic_enabled:
                        # 恢复流量的 reload 可能已生效，回滚前也要重新排空。
                        self.set_backend(port, down=True)
                        self.reload(drain=True, peer=peer)
                    log("回滚端口 {} 的二进制：{}".format(port, backup))
                    atomic_install(backup, binary)
                    self.run("systemctl", "restart", service, timeout=None)
                    self.wait_healthy(instance)
                self.write_config(original_config)
                self.reload()
                log("端口 {} 已恢复到本次更新前的状态；停止后续发布".format(port))
            except BaseException as recovery_error:
                log("自动恢复未完成：{}；请检查 {} 和 {}".format(recovery_error, service, self.config))
                raise RuntimeError("发布失败且需要人工恢复；后续实例不会更新") from recovery_error
            raise RuntimeError("端口 {} 发布失败，已恢复；后续实例不会更新".format(port)) from error

    def deploy(self):
        config_backup = self.config.with_name(self.config.name + ".mixrouter.bak." + self.release_id)
        shutil.copy2(str(self.config), str(config_backup))
        log("Nginx 配置备份：{}".format(config_backup))
        for instance, peer in ((self.instances[0], self.instances[1]), (self.instances[1], self.instances[0])):
            self.deploy_instance(instance, peer)
        log("发布完成：3000、3001 均已更新并恢复流量")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--nginx-config", default="/etc/nginx/nginx.conf")
    parser.add_argument("--nginx-pid-file", default="/run/nginx.pid")
    parser.add_argument("--drain-timeout", type=int, default=3600)
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument("--check", action="store_true")
    arguments = parser.parse_args()
    if os.geteuid() != 0:
        parser.error("请通过 root SSH 用户执行，以修改 Nginx、检查端口并重启服务")
    if arguments.drain_timeout <= 0 or arguments.health_timeout <= 0:
        parser.error("等待时间必须大于 0")

    # 让 SSH 中断和常见终止信号进入当前实例的恢复流程。
    def interrupted(number, frame):
        raise KeyboardInterrupt("收到信号 {}，停止发布".format(number))

    for number in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        signal.signal(number, interrupted)
    try:
        with open("/run/lock/mixrouter-deploy.lock", "w") as lock:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise RuntimeError("服务器已有另一个 Mixrouter 发布进程")
            deployment = Deployment(arguments)
            deployment.preflight()
            if not arguments.check:
                deployment.deploy()
    except (Exception, KeyboardInterrupt) as error:
        log("ERROR: {}".format(error))
        return 1
    finally:
        # 仅清理由本地脚本创建的专用上传目录。
        work_dir = Path(arguments.binary).parent
        if work_dir.parent == Path("/var/tmp") and re.fullmatch(r"mixrouter-deploy\.[a-zA-Z0-9]+", work_dir.name):
            shutil.rmtree(str(work_dir), ignore_errors=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
