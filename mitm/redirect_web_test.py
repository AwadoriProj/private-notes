import json
import os
import tempfile
import unittest

from mitmproxy.test import tflow, tutils

CONFIG = {
    "upstream_url": "https://127.0.0.1:9443",
    "hosts": ["biligame.net", "bilibiligame.net", "gamerfusiontech.com"],
    "routes": {"l14-prod-sg-patch-sirius.bilibiligame.net": "http://127.0.0.1:5081"},
}


def load_module():
    handle = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
    json.dump(CONFIG, handle)
    handle.close()
    os.environ["PRIVATE_NOTES_MITM_CONFIG"] = handle.name
    import importlib.util

    spec = importlib.util.spec_from_file_location(
        "redirect_web", os.path.join(os.path.dirname(__file__), "redirect_web.py")
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.load(None)
    os.unlink(handle.name)
    return module


def wireguard_flow(sni_host, ip, path=b"/app.playerlogin.PlayerLoginService/GetServerList"):
    flow = tflow.tflow()
    flow.request = tutils.treq(
        host=ip,
        port=443,
        scheme=b"https",
        authority=sni_host.encode(),
        http_version=b"HTTP/2.0",
        path=path,
        headers=((b"content-type", b"application/grpc"),),
    )
    return flow


class RedirectTest(unittest.TestCase):
    def setUp(self):
        self.module = load_module()

    def test_ip_destination_is_matched_by_authority(self):
        flow = wireguard_flow("l14-prod-va-all-gs-sirius.bilibiligame.net", "18.64.18.94")
        self.module.request(flow)
        self.assertEqual(flow.request.host, "127.0.0.1")
        self.assertEqual(flow.request.port, 9443)
        self.assertEqual(flow.request.scheme, "https")
        self.assertEqual(
            flow.request.headers["X-Private-Notes-Original-Host"],
            "l14-prod-va-all-gs-sirius.bilibiligame.net",
        )

    def test_sdk_host_is_redirected(self):
        flow = wireguard_flow("l11-sdk-login-intl.biligame.net", "18.67.175.5", b"/gapi/client/activate")
        self.module.request(flow)
        self.assertEqual((flow.request.host, flow.request.port), ("127.0.0.1", 9443))

    def test_cdn_route_wins(self):
        flow = wireguard_flow("l14-prod-sg-patch-sirius.bilibiligame.net", "18.64.18.58", b"/master/x/y.bin")
        self.module.request(flow)
        self.assertEqual((flow.request.host, flow.request.port, flow.request.scheme), ("127.0.0.1", 5081, "http"))

    def test_other_hosts_are_untouched(self):
        flow = wireguard_flow("cdp.cloud.unity3d.com", "34.107.172.168", b"/v1/events")
        self.module.request(flow)
        self.assertEqual((flow.request.host, flow.request.port), ("34.107.172.168", 443))
        self.assertNotIn("X-Private-Notes-Original-Host", flow.request.headers)


if __name__ == "__main__":
    unittest.main()
