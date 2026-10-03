import json
import os
from urllib.parse import urlparse

from mitmproxy import http

CONFIG = {}


def load(loader):
    global CONFIG
    path = os.environ["PRIVATE_NOTES_MITM_CONFIG"]
    with open(path, encoding="utf-8") as file:
        CONFIG = json.load(file)


def matches(host, patterns):
    host = host.lower().split(":", 1)[0]
    for pattern in patterns:
        pattern = pattern.lower()
        if host == pattern or host.endswith("." + pattern):
            return True
    return False


def upstream_for(config, host):
    routes = config.get("routes") or {}
    matching = [pattern for pattern in routes if matches(host, [pattern])]
    if matching:
        pattern = max(matching, key=len)
        value = routes[pattern]
        return urlparse(value if "://" in value else "https://" + value)
    if matches(host, config.get("hosts") or []):
        value = config.get("upstream_url", "")
        return urlparse(value if "://" in value else "https://" + value)
    return None


def request(flow: http.HTTPFlow):
    original_host = flow.request.host
    upstream = upstream_for(CONFIG, original_host)
    if not upstream or not upstream.hostname:
        return
    flow.request.headers["X-Private-Notes-Original-Host"] = original_host
    flow.request.host = upstream.hostname
    if upstream.port:
        flow.request.port = upstream.port
    flow.request.scheme = upstream.scheme or "https"
    flow.request.headers["Host"] = upstream.netloc
