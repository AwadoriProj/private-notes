# Our Notes Private Server
> if you got access to this source code, please don't leak it yet.
## Setup

> Make sure you have Golang installed.

Set `server_name`, `game_server_port`, and `mitm_port` in [base.config.json](./base.config.json), then run setup to generate `game/config.json` and `mitm/config.json` from those values. The optional `version`, `resource_version`, and server root fields supply the dumped gRPC services with their current endpoint and version values. Rerun setup after changing the base config.

Create db:
```bash
python scripts/setup_db.py
```

Generate CA + game TLS cert + both config.json files:
```bash
python scripts/setup.py
```

Capture:
```powershell
py -m pip install mitmproxy
./capture/run_capture.ps1
```
All captured requests and responses are also saved in one mitmproxy flow file at `capture/latest/raw`.

Generate captured.go:
```powershell
py capture/generate_captured.py
```

Update generated game protobuf bindings from the separate app-protos repository:
```powershell
py scripts/update_protos.py
```

## Usage

Run game server (TLS; port comes from `base.config.json`):
```bash
go run ./game game/config.json
```
Run assetbundle server (:5081):
```bash
go run ./assetbundle_server -config assetbundle_server/config.json
```
Set `ASSETBUNDLE_UPSTREAM_AUTHORIZATION` in that terminal before starting it. Use the `Authorization` header from a successful original asset request in mitmweb; keep the value private. The asset CDN route in `mitm/config.json` forwards to this server on `127.0.0.1:5081`.

Run MITM proxy (port comes from `base.config.json`):
```bash
go run ./mitm mitm/config.json
```
or
Run MITM proxy with mitmweb:
```bash
go run ./mitm mitm/config.json --web
```

> make sure the client device uses this machine's IP and the `mitm_port` from `base.config.json` as its HTTP(S) proxy, and that certificate pinning is disabled in the client.

## Progress
- [X] Login SDK
- [ ] Game gRPC and masterdata bootstrap
- [ ] Masterdata and on demand assets
- [ ] etc

## Note
OTP is 000000
### Requirements (for host)
- PostgreSQL
- Golang
- Python
