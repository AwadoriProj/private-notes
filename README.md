# O## N#### Private Server
A private server made for a certain rhythm game.

## Requirements

- Go 1.22 or later
- Python 3
- PostgreSQL 
- A client device

## Setup
> The command used here is for windows powershell

### Basic setup
Edit [`base.config.json`](./base.config.json). Set at least:

- `server_name`
- `game_server_port`
- `mitm_port`

You'll need to capture the actual game server to field out `version`, `resource_version`, and `cdn_root`.

Update protobuf:
```powershell
python scripts/update_protos.py
```

Create db:
```powershell
python scripts/setup_db.py
```

Setup config:
```powershell
python scripts/setup.py
```

### Extra: setup assetbundle server

Copy `assetbundle_server/config.example.json` name it `assetbundle_server/config.json`

Start capturing server reference:
```powershell
./capture/run_capture.ps1 --wireguard
```

Set env:
```powershell
$env:ASSETBUNDLE_UPSTREAM_AUTHORIZATION = (Get-Content -Raw capture/latest/upstream_authorization.txt).Trim()
```

Keep the value a secret, ok? 

Test probe catalog:
```powershell
go run ./assetbundle_server -config assetbundle_server/config.json -probe <relative-path>
```

`200` = served
`401` / `403` = unauthorized
`404` = not found

### Running the server:


asset-bundle server:
```powershell
go run ./assetbundle_server -config assetbundle_server/config.json
```

game server:
```powershell
go run ./game ./game/config.json
```

mitm:
```powershell
go run ./mitm --config ./mitm/config.json
```
> add --web flag if ya prefer using mitmweb. make sure you've atleast run it with --wireguard flag once. just once.

#### WireGuard 

Mitm redirect require wireguard for gRPC. make sure its installed on client device. make a tunnel and connect to mitm + wireguard

### Connect to client
Connect client to mitm using wireguard. theres a lot of tutorials for that if you dont know how/

## Current progress

- [x] Login SDK
- [x] (almost) Game gRPC and master-data bootstrap
- [x] Master data and on-demand assets
- [ ] Gameplay

The fixed OTP is `000000`.
