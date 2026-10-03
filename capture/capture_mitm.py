from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path

from mitmproxy import http

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(os.environ.get("CAPTURE_OUTPUT", ROOT / "capture" / "latest"))

TARGETS = [
    (re.compile(r"^/gapi/client/config/?$", re.I), "gapi-config"),
    (re.compile(r"^/gapi/client/country/list/?$", re.I), "country-list"),
    (re.compile(r"^/sdk/overseas/config/?$", re.I), "overseas-config"),
    (re.compile(r"^/gapi/client/configv2/?$", re.I), "game-support-config"),
    (re.compile(r"^/netcheck/config/safe/?$", re.I), "netcheck-safe"),
    (re.compile(r"^/app/time/conf/?$", re.I), "realtime-config"),
    (re.compile(r"^/sdk-hot-deploy/featureflag/client/config/?$", re.I), "feature-flag"),
    (re.compile(r"^/config/getconfig/?$", re.I), "cloud-storage-config"),
]


def capture_name(path: str, data: object) -> str | None:
    for pattern, name in TARGETS:
        if pattern.fullmatch(path):
            if name == "gapi-config":
                raw = json.dumps(data, ensure_ascii=False).lower()
                return "agreement-config" if "agreement_config_list" in raw else "login-config"
            return name
    return None


def request(flow: http.HTTPFlow) -> None:
    path = flow.request.path.split("?", 1)[0].lower()
    if flow.request.method in {"GET", "HEAD"} and ("/asset/" in path or "/master/" in path):
        authorization = flow.request.headers.get("authorization")
        if authorization:
            flow.metadata["private_notes_authorization"] = authorization


def response(flow: http.HTTPFlow) -> None:
    path = flow.request.path.split("?", 1)[0].lower()
    if not flow.response:
        return

    authorization = flow.metadata.get("private_notes_authorization")
    if authorization and 200 <= flow.response.status_code < 300:
        OUT.mkdir(parents=True, exist_ok=True)
        auth_path = OUT / "upstream_authorization.txt"
        tmp_auth_path = OUT / "upstream_authorization.txt.tmp"
        tmp_auth_path.write_text(authorization + "\n", encoding="utf-8")
        tmp_auth_path.replace(auth_path)
        print("captured Authorization header from successful asset/master request")

    try:
        body = flow.response.get_text(strict=False)
        data = json.loads(body)
    except (ValueError, UnicodeError):
        print(f"skip non-JSON response: {flow.request.pretty_url}", file=sys.stderr)
        return

    name = capture_name(path, data)
    if not name:
        return

    if flow.response.status_code < 200 or flow.response.status_code >= 300:
        print(f"skip HTTP {flow.response.status_code}: {flow.request.pretty_url}", file=sys.stderr)
        return

    OUT.mkdir(parents=True, exist_ok=True)
    tmp = OUT / f"{name}.json.tmp"
    tmp.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    tmp.replace(OUT / f"{name}.json")
    print(f"captured {flow.response.status_code} {path} -> {OUT / f'{name}.json'}")
