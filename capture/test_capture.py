import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path

from mitmproxy import io
from mitmproxy.test import tflow, tutils

HERE = Path(__file__).resolve().parent
AUTH = "Basic dGVzdDp0ZXN0"


def load(name, out_dir=None):
    if out_dir is not None:
        os.environ["CAPTURE_OUTPUT"] = str(out_dir)
    spec = importlib.util.spec_from_file_location(name, HERE / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def cdn_flow(path=b"/prod/en_x/asset/Android/catalog.bin", status=200, auth=AUTH, method=b"GET", host="18.64.18.58"):
    headers = ((b"authorization", auth.encode()),) if auth else ()
    request = tutils.treq(
        host=host,
        port=443,
        scheme=b"https",
        authority=b"l14-prod-sg-patch-sirius.bilibiligame.net",
        http_version=b"HTTP/2.0",
        method=method,
        path=path,
        headers=headers,
    )
    response = tutils.tresp(status_code=status, content=b"\x00\x01\x02\xff", headers=((b"content-type", b"application/octet-stream"),))
    return tflow.tflow(req=request, resp=response)


def json_flow(path, body):
    request = tutils.treq(path=path, method=b"GET")
    response = tutils.tresp(
        status_code=200,
        content=json.dumps(body).encode(),
        headers=((b"content-type", b"application/json"),),
    )
    return tflow.tflow(req=request, resp=response)


class CaptureAddonTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.out = Path(self.tmp.name)
        self.addon = load("capture_mitm", self.out)

    def tearDown(self):
        self.tmp.cleanup()

    def run_flow(self, flow):
        self.addon.request(flow)
        self.addon.response(flow)

    def test_saves_authorization_from_successful_asset_request(self):
        self.run_flow(cdn_flow())
        self.assertEqual((self.out / "upstream_authorization.txt").read_text(), AUTH + "\n")

    def test_saves_from_master_path_and_head_requests(self):
        self.run_flow(cdn_flow(path=b"/prod/en_x/master/abc/MasterCard.bin", method=b"HEAD"))
        self.assertEqual((self.out / "upstream_authorization.txt").read_text(), AUTH + "\n")

    def test_ignores_failed_requests(self):
        self.run_flow(cdn_flow(status=401))
        self.assertFalse((self.out / "upstream_authorization.txt").exists())

    def test_ignores_non_asset_paths(self):
        self.run_flow(cdn_flow(path=b"/sdk/login/ui/abTest"))
        self.assertFalse((self.out / "upstream_authorization.txt").exists())

    def test_ignores_requests_without_authorization(self):
        self.run_flow(cdn_flow(auth=""))
        self.assertFalse((self.out / "upstream_authorization.txt").exists())

    def test_same_value_is_written_once(self):
        self.assertTrue(self.addon.save_authorization(AUTH))
        self.assertFalse(self.addon.save_authorization(AUTH))
        self.assertTrue(self.addon.save_authorization("Basic b3RoZXI="))
        self.assertEqual((self.out / "upstream_authorization.txt").read_text(), "Basic b3RoZXI=\n")

    def test_binary_asset_response_is_not_treated_as_endpoint_json(self):
        self.run_flow(cdn_flow())
        self.assertEqual(sorted(p.name for p in self.out.iterdir()), ["upstream_authorization.txt"])

    def test_endpoint_json_is_still_captured(self):
        self.run_flow(json_flow(b"/sdk/overseas/config", {"a": 1}))
        self.assertEqual(json.loads((self.out / "overseas-config.json").read_text()), {"a": 1})

    def test_gapi_config_split_is_preserved(self):
        self.run_flow(json_flow(b"/gapi/client/config", {"agreement_config_list": []}))
        self.run_flow(json_flow(b"/gapi/client/config", {"other": True}))
        self.assertTrue((self.out / "agreement-config.json").exists())
        self.assertTrue((self.out / "login-config.json").exists())


class ExtractAuthorizationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)
        self.extract = load("extract_authorization")

    def tearDown(self):
        self.tmp.cleanup()

    def write_flows(self, flows):
        path = self.dir / "flows"
        with path.open("wb") as handle:
            writer = io.FlowWriter(handle)
            for flow in flows:
                writer.add(flow)
        return path

    def test_extracts_most_common_successful_value(self):
        source = self.write_flows([
            cdn_flow(auth="Basic b3RoZXI="),
            cdn_flow(),
            cdn_flow(),
            cdn_flow(status=403, auth="Basic ZmFpbGVk"),
            cdn_flow(path=b"/sdk/x"),
        ])
        output = self.dir / "out" / "upstream_authorization.txt"
        self.assertEqual(self.extract.main([str(source), "--output", str(output)]), 0)
        self.assertEqual(output.read_text(), AUTH + "\n")

    def test_returns_error_when_nothing_found(self):
        source = self.write_flows([cdn_flow(status=401), cdn_flow(auth="")])
        output = self.dir / "upstream_authorization.txt"
        self.assertEqual(self.extract.main([str(source), "--output", str(output)]), 1)
        self.assertFalse(output.exists())

    def test_returns_error_for_missing_or_invalid_file(self):
        self.assertEqual(self.extract.main([str(self.dir / "missing")]), 2)
        bad = self.dir / "bad"
        bad.write_bytes(b"not a flow file")
        self.assertEqual(self.extract.main([str(bad), "--output", str(self.dir / "o.txt")]), 2)

    def test_value_is_never_printed(self):
        import contextlib
        import io as stdio

        source = self.write_flows([cdn_flow()])
        buffer = stdio.StringIO()
        with contextlib.redirect_stdout(buffer), contextlib.redirect_stderr(buffer):
            self.extract.main([str(source), "--output", str(self.dir / "o.txt")])
        self.assertNotIn("dGVzdDp0ZXN0", buffer.getvalue())


if __name__ == "__main__":
    unittest.main()
