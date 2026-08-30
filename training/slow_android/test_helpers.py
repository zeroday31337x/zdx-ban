import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import trainer  # noqa: E402


class HelpersTest(unittest.TestCase):
    def test_atomic_state_and_hash(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            value = root / "value"
            value.write_bytes(b"abc")
            self.assertEqual(
                trainer.sha256(value),
                "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
            )
            state = root / "state.json"
            trainer.atomic_json(state, {"step": 1})
            self.assertEqual(json.loads(state.read_text()), {"step": 1})
            self.assertGreater(trainer.disk_available_mb(root), 0)

    def test_tree_hash_is_deterministic_and_path_sensitive(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "a").write_bytes(b"same")
            first = trainer.sha256_tree(root)
            self.assertEqual(first, trainer.sha256_tree(root))
            (root / "a").rename(root / "b")
            self.assertNotEqual(first, trainer.sha256_tree(root))

    def test_release_binds_dataset_and_w0(self):
        with tempfile.TemporaryDirectory() as directory:
            release = Path(directory)
            dataset = release / "w1-training.jsonl"
            dataset.write_text('{"instruction":"i","output":"o"}\n')
            manifest = {
                "release_id": "release-1",
                "training_sha256": trainer.sha256(dataset),
                "w0_sha256": "a" * 64,
            }
            (release / "w1-release.json").write_text(json.dumps(manifest))
            got, got_dataset, dataset_hash, w0_hash = trainer.load_release(release)
            self.assertEqual(got["release_id"], "release-1")
            self.assertEqual(got_dataset, dataset)
            self.assertEqual(dataset_hash, manifest["training_sha256"])
            self.assertEqual(w0_hash, manifest["w0_sha256"])
            dataset.write_text('{"instruction":"changed","output":"o"}\n')
            with self.assertRaisesRegex(RuntimeError, "has changed"):
                trainer.load_release(release)

    def test_release_without_w0_binding_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            release = Path(directory)
            dataset = release / "w1-training.jsonl"
            dataset.write_text('{"instruction":"i","output":"o"}\n')
            (release / "w1-release.json").write_text(
                json.dumps({"release_id": "release-1", "training_sha256": trainer.sha256(dataset)})
            )
            with self.assertRaisesRegex(RuntimeError, "not bound"):
                trainer.load_release(release)

    def test_corrupt_and_cross_release_state_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            state_path = output / "state.json"
            expected = trainer.initial_state("release-1", "d" * 64, "b" * 64)
            state_path.write_text("not-json")
            with self.assertRaisesRegex(RuntimeError, "corrupt"):
                trainer.load_state(state_path, expected, output)
            wrong = dict(expected)
            wrong["release_id"] = "release-2"
            trainer.atomic_json(state_path, wrong)
            with self.assertRaisesRegex(RuntimeError, "release_id"):
                trainer.load_state(state_path, expected, output)

    def test_uncheckpointed_gradients_and_tampered_checkpoint_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            state_path = output / "state.json"
            expected = trainer.initial_state("release-1", "d" * 64, "b" * 64)
            pending = dict(expected)
            pending["pending_micro_steps"] = 1
            trainer.atomic_json(state_path, pending)
            with self.assertRaisesRegex(RuntimeError, "gradients"):
                trainer.load_state(state_path, expected, output)

            checkpoint = output / "checkpoint-00000000"
            checkpoint.mkdir()
            (checkpoint / "adapter.safetensors").write_bytes(b"valid")
            saved = dict(expected)
            saved["checkpoint"] = checkpoint.name
            saved["checkpoint_sha256"] = trainer.sha256_tree(checkpoint)
            trainer.atomic_json(state_path, saved)
            trainer.load_state(state_path, expected, output)
            (checkpoint / "adapter.safetensors").write_bytes(b"tampered")
            with self.assertRaisesRegex(RuntimeError, "hash"):
                trainer.load_state(state_path, expected, output)

    def test_retryable_preflight_requires_only_transient_issues(self):
        transient = trainer.PreflightResult([trainer.PreflightIssue("memory", retryable=True)])
        self.assertTrue(transient.retryable)
        mixed = trainer.PreflightResult(
            [trainer.PreflightIssue("memory", retryable=True), trainer.PreflightIssue("release")]
        )
        self.assertFalse(mixed.retryable)

    def test_config_validation(self):
        config = json.loads((Path(__file__).with_name("config.json")).read_text())
        self.assertIs(trainer.validate_config(config), config)
        config["gradient_accumulation_steps"] = 0
        with self.assertRaisesRegex(RuntimeError, "positive integer"):
            trainer.validate_config(config)

    def test_encoded_example_keeps_response_labels(self):
        class FakeTensor:
            def __init__(self, values):
                self.values = values

        class FakeTorch:
            long = "long"

            @staticmethod
            def tensor(values, dtype=None):
                return FakeTensor(values)

            @staticmethod
            def ones(shape, dtype=None):
                return FakeTensor([[1] * shape[1]])

        class FakeTokenizer:
            eos_token = "<eos>"
            eos_token_id = 99

            def __call__(self, text, add_special_tokens=False):
                return {"input_ids": list(range(1, len(text.split()) + 1))}

        encoded = trainer.encoded_example(
            FakeTokenizer(),
            {"instruction": "one two three four", "output": "answer"},
            4,
            FakeTorch,
        )
        labels = encoded["labels"].values[0]
        self.assertEqual(len(labels), 4)
        self.assertTrue(any(value != -100 for value in labels))


if __name__ == "__main__":
    unittest.main()
