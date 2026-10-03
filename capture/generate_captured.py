from __future__ import annotations

import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SOURCE_NAMES = {
    "loginConfigJSON": "login-config.json",
    "countryListJSON": "country-list.json",
    "overseasConfigJSON": "overseas-config.json",
    "gameSupportConfigJSON": "game-support-config.json",
    "agreementConfigJSON": "agreement-config.json",
    "netcheckSafeData": "netcheck-safe.json",
    "realtimeConfJSON": "realtime-config.json",
    "featureFlagJSON": "feature-flag.json",
    "cloudStorageJSON": "cloud-storage-config.json",
}
def unwrap(name: str, value: object) -> object:
    if name in {
        "loginConfigJSON",
        "countryListJSON",
        "overseasConfigJSON",
        "agreementConfigJSON",
        "gameSupportConfigJSON",
        "featureFlagJSON",
        "cloudStorageJSON",
        "netcheckSafeData",
    } and isinstance(value, dict) and "data" in value:
        return value["data"]
    return value


def go_raw(value: object) -> str:
    encoded = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    if "`" in encoded:
        raise ValueError("JSON value contains a backtick; cannot emit a Go raw string literal")
    return f"json.RawMessage(`{encoded}`)"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, default=ROOT / "capture" / "latest")
    parser.add_argument("--output", type=Path, default=ROOT / "game" / "config" / "captured.go")
    parser.add_argument("--snapshot", type=Path, default=ROOT / "capture" / "latest" / "captured.json")
    args = parser.parse_args()

    values: dict[str, object] = {}
    missing: list[str] = []
    for variable, filename in SOURCE_NAMES.items():
        path = args.input / filename
        if not path.is_file():
            missing.append(filename)
            continue
        values[variable] = unwrap(variable, json.loads(path.read_text(encoding="utf-8")))

    if missing:
        raise SystemExit("missing captures (run the client through capture proxy first): " + ", ".join(missing))

    source = "package config\n\nimport \"encoding/json\"\n\n"
    for variable, value in values.items():
        source += f"var {variable} = {go_raw(value)}\n\n"

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(source, encoding="utf-8")
    args.snapshot.parent.mkdir(parents=True, exist_ok=True)
    args.snapshot.write_text(json.dumps(values, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {args.output}")
    print(f"wrote {args.snapshot}")


if __name__ == "__main__":
    main()
