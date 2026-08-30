import json
import subprocess
import tempfile
import time
import unittest
from pathlib import Path


HERE = Path(__file__).resolve().parent
WATCHDOG = HERE / "watchdog.sh"


class UbuntuWatchdogTest(unittest.TestCase):
    def make_env(self, root: Path, output: Path, enabled: bool, stage: str = "W1") -> Path:
        for relative in ("training/slow_android/trainer.py", "training/w0/trainer.py"):
            trainer = root / relative
            trainer.parent.mkdir(parents=True, exist_ok=True)
            trainer.write_text("raise SystemExit(2)\n", encoding="utf-8")
        env_file = root / "training.env"
        env_file.write_text(
            "\n".join(
                [
                    f"ZDX_TRAINING_ENABLED={int(enabled)}",
                    f"ZDX_TRAINING_STAGE={stage}",
                    f"ZDX_PROJECT_ROOT={root}",
                    "ZDX_PYTHON=/usr/bin/python3",
                    f"ZDX_W1_RELEASE={root / 'release'}",
                    f"ZDX_W0_RELEASE={root / 'w0-release'}",
                    f"ZDX_W0_TRANSFORMERS={root / 'w0'}",
                    f"ZDX_SLOW_OUTPUT={output}",
                    f"ZDX_W1_CONFIG={root / 'config.json'}",
                    "",
                ]
            ),
            encoding="utf-8",
        )
        return env_file

    def run_watchdog(self, env_file: Path) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(WATCHDOG), str(env_file)],
            check=False,
            capture_output=True,
            text=True,
            timeout=5,
        )

    def test_disabled_configuration_is_inert(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "output"
            result = self.run_watchdog(self.make_env(root, output, enabled=False))
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse(output.exists())

    def test_completed_state_is_not_restarted(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "output"
            output.mkdir()
            (output / "state.json").write_text(
                json.dumps({"status": "TRAINING_COMPLETE_UNVALIDATED"}), encoding="utf-8"
            )
            result = self.run_watchdog(self.make_env(root, output, enabled=True))
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((output / "training.log").exists())

    def test_permanent_trainer_failure_latches_restart(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "output"
            result = self.run_watchdog(self.make_env(root, output, enabled=True))
            self.assertEqual(result.returncode, 0, result.stderr)
            deadline = time.monotonic() + 3
            while time.monotonic() < deadline and not (output / "PERMANENT_FAILURE").exists():
                time.sleep(0.02)
            self.assertTrue((output / "PERMANENT_FAILURE").exists())
            second = self.run_watchdog(root / "training.env")
            self.assertEqual(second.returncode, 0)
            self.assertIn("restart is latched", second.stderr)

    def test_w0_stage_uses_the_same_failure_latch(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "w0-output"
            result = self.run_watchdog(self.make_env(root, output, enabled=True, stage="W0"))
            self.assertEqual(result.returncode, 0, result.stderr)
            deadline = time.monotonic() + 3
            while time.monotonic() < deadline and not (output / "PERMANENT_FAILURE").exists():
                time.sleep(0.02)
            self.assertTrue((output / "PERMANENT_FAILURE").exists())


if __name__ == "__main__":
    unittest.main()
