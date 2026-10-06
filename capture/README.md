## Prerequisites

- Python 3.10+
- mitmproxy (`py -m pip install mitmproxy`)
- Test device

## Capture

From the repository root, run:

```powershell
py -m pip install mitmproxy
./capture/run_capture.ps1 --web
```

Set the device proxy to this computer on port `8080`, install the mitmproxy CA, and make at least one successful original server request under `/asset/` or `/master/`.

The game's gRPC connection ignores the device proxy setting, so capture it through WireGuard instead:

```powershell
./capture/run_capture.ps1 --web --wireguard
```

This serves the regular proxy on port `8080` and WireGuard on UDP `51820` together (`--wireguard-port` changes the WireGuard port). Scan the WireGuard QR code in mitmweb with the WireGuard app on the device and turn the tunnel on. If the Endpoint is not this computer's LAN IP, edit it in the WireGuard app. The proxy CA must be trusted by the device in both modes.

The addon saves the Authorization header from the first successful `/asset/` or `/master/` request and only rewrites the file when the value changes. The addon saves that request's Authorization header to `capture/latest/upstream_authorization.txt` without printing its value. This file is ignored by Git.

To use the captured value for the assetbundle server in PowerShell, load it in the same terminal before starting the server:

```powershell
$env:ASSETBUNDLE_UPSTREAM_AUTHORIZATION = (Get-Content -Raw capture/latest/upstream_authorization.txt).Trim()
go run ./assetbundle_server -config assetbundle_server/config.json
```

## Extract the Authorization header from an existing capture

If a flow file already exists, for example one saved from mitmweb, no new capture is needed:

```powershell
py capture/extract_authorization.py capture/latest/raw
```

Pass another flow file path as the argument. The script writes `capture/latest/upstream_authorization.txt` (override with `--output`) and prints only the scheme, the length, and how many requests carried the value, never the value itself. It saves the most common value from successful `GET` and `HEAD` requests under `/asset/` or `/master/`, and exits with a non-zero code if none exist.

## Tests

```powershell
py -m unittest capture.test_capture
```

## Generate files

```powershell
py capture/generate_captured.py
```

The proxy also saves every captured flow, including requests and responses, as one mitmproxy flow file at `capture/latest/raw`. Open it in mitmweb with `mitmweb -r capture/latest/raw` or load it with `mitmproxy -r capture/latest/raw`.

The addon saves each selected response under `capture/latest/` as endpoint JSON. The generator validates that every required response was captured, writes a combined `captured.json` snapshot, and updates `game/config/captured.go`. Set `--input` to use another capture directory. Review the generated files before committing.
