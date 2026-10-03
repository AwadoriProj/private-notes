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

Set the device proxy to this computer on port `8080`, install the mitmproxy CA, and make at least one successful original server request under `/asset/` or `/master/`. The addon saves that request's Authorization header to `capture/latest/upstream_authorization.txt` without printing its value. This file is ignored by Git.

To use the captured value for the assetbundle server in PowerShell, load it in the same terminal before starting the server:

```powershell
$env:ASSETBUNDLE_UPSTREAM_AUTHORIZATION = (Get-Content -Raw capture/latest/upstream_authorization.txt).Trim()
go run ./assetbundle_server -config assetbundle_server/config.json
```

## Generate files

```powershell
py capture/generate_captured.py
```

The proxy also saves every captured flow, including requests and responses, as one mitmproxy flow file at `capture/latest/raw`. Open it in mitmweb with `mitmweb -r capture/latest/raw` or load it with `mitmproxy -r capture/latest/raw`.

The addon saves each selected response under `capture/latest/` as endpoint JSON. The generator validates that every required response was captured, writes a combined `captured.json` snapshot, and updates `game/config/captured.go`. Set `--input` to use another capture directory. Review the generated files before committing.
