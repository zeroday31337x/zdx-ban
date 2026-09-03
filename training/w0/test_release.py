import contextlib
import json
import sys
import tempfile
import unittest
from pathlib import Path


@contextlib.contextmanager
def _isolated_import_path(directory: Path):
    """Scope sys.path/sys.modules so a same-named sibling ``trainer`` module
    elsewhere under training/ can't be cached and reused across test files."""
    directory_str = str(directory)
    sys.path.insert(0, directory_str)
    try:
        yield
    finally:
        sys.path.remove(directory_str)
        sys.modules.pop("trainer", None)


with _isolated_import_path(Path(__file__).resolve().parent):
    import release  # noqa: E402
    import prepare_release  # noqa: E402
    import trainer  # noqa: E402
    from validate_dataset import content_sha256  # noqa: E402


class FakeTokenizer:
    pad_token_id = 0
    eos_token_id = 2

    def __call__(self, text, add_special_tokens=False):
        return {"input_ids": [3 + index for index, _ in enumerate(text.split())]}


class W0ReleaseTest(unittest.TestCase):
    def test_train_config_validation(self):
        config = json.loads((Path(__file__).with_name("train-config.json")).read_text())
        self.assertIs(release.validate_train_config(config), config)
        config["torch_dtype"] = "float16"
        with self.assertRaisesRegex(RuntimeError, "float32"):
            release.validate_train_config(config)

    def test_packed_cursor_resumes_exactly(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            text = "one two three four five six seven eight"
            item = {"text": text, "split": "train"}
            shard = root / "part.jsonl"
            shard.write_text(json.dumps(item) + "\n")
            shards = [{"path": shard.name, "sha256": release.sha256(shard)}]
            cursor = {"epoch": 0, "shard_index": 0, "byte_offset": 0, "pending_tokens": []}
            first = trainer.next_block(root, shards, FakeTokenizer(), cursor, 4, 1)
            saved = json.loads(json.dumps(cursor))
            second = trainer.next_block(root, shards, FakeTokenizer(), cursor, 4, 1)
            resumed = trainer.next_block(root, shards, FakeTokenizer(), saved, 4, 1)
            self.assertEqual(first, [3, 4, 5, 6, 7])
            self.assertEqual(second, resumed)
            self.assertEqual(len(second), 5)

    def test_state_rejects_release_drift_and_pending_gradients(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            manifest = {"release_id": "w0-test", "manifest_sha256": "a" * 64}
            expected = trainer.initial_state(manifest)
            state = dict(expected)
            state["pending_micro_steps"] = 1
            release.atomic_json(output / "state.json", state)
            with self.assertRaisesRegex(RuntimeError, "uncheckpointed"):
                trainer.load_state(output / "state.json", expected, output, 1)
            state = trainer.initial_state(manifest)
            state["release_manifest_sha256"] = "b" * 64
            release.atomic_json(output / "state.json", state)
            with self.assertRaisesRegex(RuntimeError, "does not match"):
                trainer.load_state(output / "state.json", expected, output, 1)

    def test_release_integrity_binds_all_artifacts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tokenizer = root / "tokenizer"
            tokenizer.mkdir()
            (tokenizer / "tokenizer.json").write_text("{}")
            model_config = root / "model-config.json"
            model = json.loads(Path(__file__).with_name("model-template.json").read_text())
            model.update({"vocab_size": 128, "pad_token_id": 0, "bos_token_id": 1, "eos_token_id": 2})
            model_config.write_text(json.dumps(model))
            train_config = root / "train-config.json"
            train_config.write_text(Path(__file__).with_name("train-config.json").read_text())
            text = "A sufficiently long validated training document for the fixture. " * 4
            shard = root / "part.jsonl"
            item = {
                "id": "one",
                "text": text,
                "source": "fixture",
                "source_type": "test",
                "language": "en",
                "license": "fixture",
                "split": "train",
                "synthetic": False,
                "content_sha256": content_sha256(text),
            }
            shard.write_text(json.dumps(item) + "\n")
            report = root / "validation-report.json"
            report.write_text(json.dumps({"valid": True, "by_split": {"train": 1}}))
            manifest = {
                "schema_version": 1,
                "release_id": "w0-test",
                "corpus": {
                    "shards": [{"path": shard.name, "sha256": release.sha256(shard)}],
                    "validation_report": report.name,
                    "validation_report_sha256": release.sha256(report),
                },
                "artifacts": {
                    "tokenizer": {"path": tokenizer.name, "sha256": release.sha256_tree(tokenizer)},
                    "model_config": {"path": model_config.name, "sha256": release.sha256(model_config)},
                    "train_config": {"path": train_config.name, "sha256": release.sha256(train_config)},
                },
            }
            release.atomic_json(root / "w0-release.json", manifest)
            loaded = release.load_release(root)
            self.assertEqual(loaded["release_id"], "w0-test")
            shard.write_text("changed")
            with self.assertRaisesRegex(RuntimeError, "has changed"):
                release.load_release(root)

    def test_prepare_release_binds_validated_corpus_and_tokenizer(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            corpus = root / "corpus"
            corpus.mkdir()
            shard = corpus / "part.jsonl"
            text = "A reviewed corpus record used only for release assembly testing. " * 4
            item = {
                "id": "document-1",
                "text": text,
                "source": "fixture",
                "source_type": "test",
                "language": "en",
                "license": "fixture",
                "split": "train",
                "synthetic": False,
                "content_sha256": content_sha256(text),
            }
            shard.write_text(json.dumps(item) + "\n")
            shard_hash = release.sha256(shard)
            report = root / "source-report.json"
            report.write_text(
                json.dumps(
                    {
                        "valid": True,
                        "by_split": {"train": 1},
                        "files": [{"path": str(shard.resolve()), "sha256": shard_hash, "records": 1}],
                    }
                )
            )
            tokenizer = root / "tokenizer"
            tokenizer.mkdir()
            (tokenizer / "tokenizer.json").write_text("{}")
            release.atomic_json(
                tokenizer / "tokenizer-metadata.json",
                {
                    "vocab_size": 128,
                    "special_token_ids": {"pad_token_id": 0, "bos_token_id": 1, "eos_token_id": 2, "unk_token_id": 3},
                    "corpus_shards": [{"path": str(shard), "sha256": shard_hash}],
                },
            )
            manifest = prepare_release.prepare(
                root,
                "w0-prepared",
                report,
                tokenizer,
                Path(__file__).with_name("model-template.json"),
                Path(__file__).with_name("train-config.json"),
            )
            self.assertEqual(manifest["release_id"], "w0-prepared")
            loaded = release.load_release(root)
            self.assertEqual(loaded["release_id"], "w0-prepared")


if __name__ == "__main__":
    unittest.main()
