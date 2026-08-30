import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import validate_dataset as validator  # noqa: E402


def record(identifier: str, text: str, **changes) -> dict:
    value = {
        "id": identifier,
        "text": text,
        "source": "fixture",
        "source_type": "documentation",
        "language": "en",
        "license": "fixture-only",
        "split": "train",
        "synthetic": False,
        "content_sha256": validator.content_sha256(text),
    }
    value.update(changes)
    return value


class DatasetValidatorTest(unittest.TestCase):
    def run_validation(self, values: list[dict], denied: set[str] | None = None) -> dict:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            shard = root / "part.jsonl"
            shard.write_text("".join(json.dumps(value) + "\n" for value in values), encoding="utf-8")
            return validator.validate(
                [shard],
                min_chars=1,
                max_bytes=4096,
                deny_hashes=denied or set(),
                index_path=root / "index.sqlite3",
                max_errors=20,
            )

    def test_valid_real_and_synthetic_records(self):
        synthetic = record(
            "synth-1",
            "A checked synthetic textbook passage.",
            synthetic=True,
            generator="generator/model@revision",
            generation_prompt_hash="a" * 64,
        )
        report = self.run_validation([record("real-1", "A licensed real passage."), synthetic])
        self.assertTrue(report["valid"])
        self.assertEqual(report["records"], 2)
        self.assertEqual(report["accepted_records"], 2)
        self.assertEqual(report["synthetic_records"], 1)

    def test_duplicate_id_and_content_are_rejected(self):
        text = "The same document appears twice."
        report = self.run_validation([record("duplicate", text), record("duplicate", text)])
        self.assertFalse(report["valid"])
        self.assertEqual(report["duplicate_ids"], 1)
        self.assertEqual(report["duplicate_contents"], 1)
        self.assertEqual(report["accepted_records"], 1)

    def test_hash_mismatch_and_missing_synthetic_provenance_are_rejected(self):
        value = record("synth", "Generated material.", synthetic=True, content_sha256="b" * 64)
        report = self.run_validation([value])
        self.assertFalse(report["valid"])
        self.assertIn("does not match", report["errors"][0]["error"])
        self.assertIn("no generator", report["errors"][0]["error"])

    def test_denied_evaluation_content_is_rejected(self):
        value = record("contaminated", "Reserved evaluation example.")
        report = self.run_validation([value], {value["content_sha256"]})
        self.assertFalse(report["valid"])
        self.assertEqual(report["denied_contents"], 1)
        self.assertEqual(report["accepted_records"], 0)

    def test_canonical_normalization_is_required(self):
        decomposed = "Cafe\u0301"
        report = self.run_validation([record("unicode", decomposed)])
        self.assertFalse(report["valid"])
        self.assertIn("not canonical", report["errors"][0]["error"])


if __name__ == "__main__":
    unittest.main()
