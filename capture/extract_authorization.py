from __future__ import annotations

import argparse
import os
import sys
from collections import Counter
from pathlib import Path

from mitmproxy import exceptions, http, io

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_INPUT = ROOT / "capture" / "latest" / "raw"
DEFAULT_OUTPUT = ROOT / "capture" / "latest" / "upstream_authorization.txt"
ASSET_MARKERS = ("/asset/", "/master/")


def collect_authorizations(path: Path) -> Counter[str]:
    found: Counter[str] = Counter()
    with path.open("rb") as handle:
        for flow in io.FlowReader(handle).stream():
            if not isinstance(flow, http.HTTPFlow) or flow.response is None:
                continue
            if flow.request.method not in {"GET", "HEAD"}:
                continue
            request_path = flow.request.path.split("?", 1)[0].lower()
            if not any(marker in request_path for marker in ASSET_MARKERS):
                continue
            if not 200 <= flow.response.status_code < 300:
                continue
            value = flow.request.headers.get("authorization")
            if value:
                found[value] += 1
    return found


def write_secret(path: Path, value: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(value + "\n", encoding="utf-8")
    try:
        os.chmod(tmp, 0o600)
    except OSError:
        pass
    tmp.replace(path)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Save the upstream asset Authorization header from an existing mitmproxy flow file."
    )
    parser.add_argument("input", nargs="?", type=Path, default=DEFAULT_INPUT, help="mitmproxy flow file")
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args(argv)

    if not args.input.is_file():
        print(f"flow file not found: {args.input}", file=sys.stderr)
        return 2
    try:
        found = collect_authorizations(args.input)
    except exceptions.FlowReadException as exc:
        print(f"cannot read {args.input} as a mitmproxy flow file: {exc}", file=sys.stderr)
        return 2
    if not found:
        print(
            "no successful GET/HEAD request under /asset/ or /master/ with an Authorization header was found",
            file=sys.stderr,
        )
        return 1

    value, count = found.most_common(1)[0]
    write_secret(args.output, value)
    scheme = value.split(" ", 1)[0] if " " in value else "(no scheme)"
    print(f"saved {scheme} Authorization header ({len(value)} characters, seen in {count} requests) to {args.output}")
    if len(found) > 1:
        print(f"warning: {len(found)} different values were found, the most common one was saved", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
