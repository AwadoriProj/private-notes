# Our Notes Private Server
> if you got access to this source code, please don't leak it yet.
## Usage
> make sure you have golang installed.
```python scripts/setup_db.py``` // Create db
```python scripts/setup.py``` // Generate CA + game TLS cert + both config.json files
```go run ./game game/config.json```   // :9443, TLS
```go run ./mitm mitm/config.json``` // :8443, MITM