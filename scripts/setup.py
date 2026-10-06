import datetime
import ipaddress
import json
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID

ROOT = Path(__file__).resolve().parent.parent
CA_DIR = ROOT / "mitm" / "ca"
GAME_DIR = ROOT / "game"

TARGET_HOSTS = [
    "biligame.net",
    "bilibiligame.net",
    "gamerfusiontech.com",
]


DEFAULT_SERVER_NAME = "Private Notes"


def load_base_config():
    path = ROOT / "base.config.json"
    cfg = json.loads(path.read_text(encoding="utf-8"))
    for key in ("game_server_port", "mitm_port"):
        value = cfg.get(key)
        if isinstance(value, bool) or not isinstance(value, int) or not 1 <= value <= 65535:
            raise ValueError(f"{key} in {path} must be an integer from 1 to 65535")
    wireguard = cfg.get("wireguard", False)
    if not isinstance(wireguard, bool):
        raise ValueError(f"wireguard in {path} must be true or false")
    wireguard_port = cfg.get("wireguard_port", 51820)
    if isinstance(wireguard_port, bool) or not isinstance(wireguard_port, int) or not 1 <= wireguard_port <= 65535:
        raise ValueError(f"wireguard_port in {path} must be an integer from 1 to 65535")
    server_id = cfg.get("server_id", 1)
    if isinstance(server_id, bool) or not isinstance(server_id, int) or server_id < 1:
        raise ValueError(f"server_id in {path} must be a positive integer")
    for key in ("display_name", "players_path", "cp_server_name"):
        if not isinstance(cfg.get(key, ""), str):
            raise ValueError(f"{key} in {path} must be a string")
    ng_words = cfg.get("ng_words", [])
    if not isinstance(ng_words, list) or not all(isinstance(word, str) for word in ng_words):
        raise ValueError(f"ng_words in {path} must be a list of strings")
    name = cfg.get("server_name", DEFAULT_SERVER_NAME)
    if not isinstance(name, str) or not name.strip():
        raise ValueError(f"server_name in {path} must be a non-empty string")
    return cfg


def generate_ca():
    CA_DIR.mkdir(parents=True, exist_ok=True)
    key_path = CA_DIR / "ca.key"
    cert_path = CA_DIR / "ca.crt"
    if key_path.exists() and cert_path.exists():
        print(f"CA already exists at {CA_DIR}, skipping")
        return cert_path, key_path

    key = rsa.generate_private_key(public_exponent=65537, key_size=4096)
    subject = issuer = x509.Name([
        x509.NameAttribute(NameOID.COMMON_NAME, "private-notes MITM CA"),
    ])
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.now(datetime.timezone.utc))
        .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=3650))
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .sign(key, hashes.SHA256())
    )

    key_path.write_bytes(
        key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.TraditionalOpenSSL,
            encryption_algorithm=serialization.NoEncryption(),
        )
    )
    cert_path.write_bytes(cert.public_bytes(serialization.Encoding.PEM))
    print(f"CA written to {CA_DIR}")
    print("Install mitm/ca/ca.crt on the test device as a trusted root certificate.")
    return cert_path, key_path


def generate_game_tls_cert():
    key_path = GAME_DIR / "server.key"
    cert_path = GAME_DIR / "server.crt"
    if key_path.exists() and cert_path.exists():
        print(f"game TLS cert already exists at {GAME_DIR}, skipping")
        return cert_path, key_path

    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    subject = issuer = x509.Name([
        x509.NameAttribute(NameOID.COMMON_NAME, "private-notes-game"),
    ])
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.now(datetime.timezone.utc))
        .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=3650))
        .add_extension(
            x509.SubjectAlternativeName([x509.IPAddress(ipaddress.ip_address("127.0.0.1"))]),
            critical=False,
        )
        .sign(key, hashes.SHA256())
    )

    key_path.write_bytes(
        key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.TraditionalOpenSSL,
            encryption_algorithm=serialization.NoEncryption(),
        )
    )
    cert_path.write_bytes(cert.public_bytes(serialization.Encoding.PEM))
    print(f"game TLS cert written to {GAME_DIR}")
    return cert_path, key_path


def write_mitm_config(ca_cert_path, ca_key_path, listen_addr, upstream_url, wireguard=False, wireguard_port=51820):
    cfg = {
        "listen_addr": listen_addr,
        "ca_cert_path": str(ca_cert_path),
        "ca_key_path": str(ca_key_path),
        "upstream_url": upstream_url,
        "hosts": TARGET_HOSTS,
        "routes": {
            "l14-prod-sg-patch-sirius.bilibiligame.net": "http://127.0.0.1:5081",
        },
        "wireguard": wireguard,
        "wireguard_port": wireguard_port,
    }
    out = ROOT / "mitm" / "config.json"
    out.write_text(json.dumps(cfg, indent=2))
    print(f"wrote {out}")


def write_game_config(listen_addr, tls_cert_path, tls_key_path, server_name):
    base_cfg = load_base_config()
    cfg = {
        "listen_addr": listen_addr,
        "tls_cert": str(tls_cert_path),
        "tls_key": str(tls_key_path),
        "rsa_key_path": str(GAME_DIR / "login_rsa.pem"),
        "version": base_cfg.get("version", ""),
        "resource_version": base_cfg.get("resource_version", ""),
        "players_path": base_cfg.get("players_path", ""),
        "ng_words": base_cfg.get("ng_words", []),
        "cp_server_name": base_cfg.get("cp_server_name", "global server"),
        "servers": [
            {
                "id": base_cfg.get("server_id", 1),
                "name": server_name,
                "display_name": base_cfg.get("display_name", "").strip() or server_name,
                "cdn_root": base_cfg.get("cdn_root", ""),
                "api_server_root": base_cfg.get("api_server_root", ""),
                "chat_server_root": base_cfg.get("chat_server_root", ""),
                "at_server_root": base_cfg.get("at_server_root", ""),
                "live_server": base_cfg.get("live_server", ""),
                "area_id": base_cfg.get("area_id", ""),
            },
        ],
    }
    db_json = GAME_DIR / "db.json"
    if db_json.exists():
        dsn = json.loads(db_json.read_text()).get("dsn", "")
        if dsn:
            cfg["db_dsn"] = dsn
            print(f"merged dsn from {db_json} as db_dsn")
    else:
        print("no game/db.json found, sessions will stay in memory; run scripts/setup_db.py then re-run this script")
    out = GAME_DIR / "config.json"
    out.write_text(json.dumps(cfg, indent=2))
    print(f"wrote {out}")


def main():
    base_cfg = load_base_config()
    game_port = base_cfg["game_server_port"]
    mitm_port = base_cfg["mitm_port"]
    server_name = base_cfg.get("server_name", DEFAULT_SERVER_NAME).strip()

    ca_cert_path, ca_key_path = generate_ca()
    game_cert_path, game_key_path = generate_game_tls_cert()
    write_mitm_config(
        ca_cert_path,
        ca_key_path,
        listen_addr=f":{mitm_port}",
        upstream_url=f"https://127.0.0.1:{game_port}",
        wireguard=base_cfg.get("wireguard", False),
        wireguard_port=base_cfg.get("wireguard_port", 51820),
    )
    write_game_config(
        listen_addr=f":{game_port}",
        tls_cert_path=game_cert_path,
        tls_key_path=game_key_path,
        server_name=server_name,
    )
    if base_cfg.get("wireguard", False):
        route = (
            f"  2. Run step 4, copy the WireGuard client config that mitmweb prints, set Endpoint to this machine's LAN IP, port {base_cfg.get('wireguard_port', 51820)}/udp, and import it into the WireGuard app on the device.\n"
            "     The game's gRPC traffic only reaches the private server through WireGuard.\n"
        )
        mitm_cmd = "go run ./mitm ./mitm/config.json"
    else:
        route = f"  2. Set the device's HTTP(S) proxy to this machine's IP, port {mitm_port}.\n"
        mitm_cmd = "go run ./mitm ./mitm/config.json"
    print(
        "\nNext you'll just:\n"
        "  1. Install mitm/ca/ca.crt on the client device, or else.\n"
        + route
        + "  3. run this : 'go run ./game ./game/config.json'\n"
        + f"  4. run this : '{mitm_cmd}'\n"
    )


if __name__ == "__main__":
    main()
