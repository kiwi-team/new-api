import importlib.util
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("import_channels.py")
SPEC = importlib.util.spec_from_file_location("import_channels", SCRIPT_PATH)
import_channels = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(import_channels)


class ImportChannelsTest(unittest.TestCase):
    def setUp(self):
        import_channels.AWS_ACCESS_KEY_ID = "ak"
        import_channels.AWS_SECRET_ACCESS_KEY = "sk"

    def test_normalize_model(self):
        cases = {
            "global.anthropic.claude-fable-5-1": "claude-fable-5-1",
            "us.anthropic.claude-haiku-4-5-20251001-v1:0": "claude-haiku-4-5-20251001",
            "global.anthropic.claude-opus-4-5-20251101-v2:0": "claude-opus-4-5-20251101",
            "global.anthropic.claude-opus-4-6-v1": "claude-opus-4-6",
            "global.anthropic.claude-opus-4-7-v2": "claude-opus-4-7",
            "eu.anthropic.claude-opus-5-v3:0": "claude-opus-5",
        }
        for raw, expected in cases.items():
            with self.subTest(raw=raw):
                self.assertEqual(import_channels.normalize_model(raw), expected)

    def test_duplicate_models_create_parallel_channels_per_arena(self):
        rows = [
            {"model": "claude-opus-4-7", "arena": "sa-east-1", "arn": "arn:1"},
            {"model": "claude-opus-4-8", "arena": "sa-east-1", "arn": "arn:2"},
            {"model": "claude-opus-4-8", "arena": "sa-east-1", "arn": "arn:3"},
            {"model": "claude-opus-4-7", "arena": "sa-east-1", "arn": "arn:4"},
        ]

        payloads = import_channels.aggregate_rows(rows)

        self.assertEqual(len(payloads), 2)
        self.assertEqual(payloads[0]["channel"]["name"], "AWS Claude sa-east-1 #1")
        self.assertEqual(payloads[1]["channel"]["name"], "AWS Claude sa-east-1 #2")
        self.assertEqual(
            payloads[0]["channel"]["models"], "claude-opus-4-7,claude-opus-4-8"
        )
        self.assertEqual(
            payloads[1]["channel"]["model_mapping"],
            '{"claude-opus-4-8":"arn:3","claude-opus-4-7":"arn:4"}',
        )
        self.assertEqual(payloads[0]["channel"]["key"], "ak|sk|sa-east-1")


if __name__ == "__main__":
    unittest.main()
