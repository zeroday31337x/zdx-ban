#!/usr/bin/env python3
import argparse
import json
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    state = args.output / "state.json"
    if not state.exists():
        print(json.dumps({"status": "NOT_STARTED"}, indent=2))
        return 0
    try:
        value = json.loads(state.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        print(json.dumps({"status": "CORRUPT_STATE", "error": str(exc)}, indent=2))
        return 2
    print(json.dumps(value, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
