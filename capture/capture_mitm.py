from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path

from mitmproxy import http

AUTH_METADATA = "private_notes_authorization"
ASSET_MARKERS = ("/asset/", "/master/")

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


saved_authorization: str | None = None


def target_name(path: str) -> str | None:
    for pattern, name in TARGETS:
        if pattern.fullmatch(path):
            return name
    return None


def capture_name(path: str, data: object) -> str | None:
    name = target_name(path)
    if name == "gapi-config":
        raw = json.dumps(data, ensure_ascii=False).lower()
        return "agreement-config" if "agreement_config_list" in raw else "login-config"
    return name


def is_asset_request(flow: http.HTTPFlow) -> bool:
    path = flow.request.path.split("?", 1)[0].lower()
    return flow.request.method in {"GET", "HEAD"} and any(marker in path for marker in ASSET_MARKERS)


def save_authorization(authorization: str) -> bool:
    global saved_authorization
    if authorization == saved_authorization:
        return False
    OUT.mkdir(parents=True, exist_ok=True)
    tmp = OUT / "upstream_authorization.txt.tmp"
    tmp.write_text(authorization + "\n", encoding="utf-8")
    tmp.replace(OUT / "upstream_authorization.txt")
    saved_authorization = authorization
    return True


def request(flow: http.HTTPFlow) -> None:
    if not is_asset_request(flow):
        return
    authorization = flow.request.headers.get("authorization")
    if authorization:
        flow.metadata[AUTH_METADATA] = authorization


def response(flow: http.HTTPFlow) -> None:
    if not flow.response:
        return

    authorization = flow.metadata.get(AUTH_METADATA)
    if authorization and 200 <= flow.response.status_code < 300 and save_authorization(authorization):
        print(f"captured Authorization header from successful asset/master request, saved to {OUT / 'upstream_authorization.txt'}")

    path = flow.request.path.split("?", 1)[0].lower()
    if target_name(path) is None:
        return

    try:
        data = json.loads(flow.response.get_text(strict=False))
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
