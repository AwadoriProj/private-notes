# Our Notes Private Server
> if you got access to this source code, please don't leak it yet.
## Usage

> Make sure you have Golang installed.

Create db:
```bash
python scripts/setup_db.py
```

Generate CA + game TLS cert + both config.json files:
```bash
python scripts/setup.py
```

Run game server (:9443, TLS):
```bash
go run ./game game/config.json
```

Run MITM proxy (:8443, MITM):
```bash
go run ./mitm mitm/config.json
```