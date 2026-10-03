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


def load_base_config():
    path = ROOT / "base.config.json"
    cfg = json.loads(path.read_text(encoding="utf-8"))
    for key in ("game_server_port", "mitm_port"):
        value = cfg.get(key)
        if isinstance(value, bool) or not isinstance(value, int) or not 1 <= value <= 65535:
            raise ValueError(f"{key} in {path} must be an integer from 1 to 65535")
    name = cfg.get("server_name")
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


def write_mitm_config(ca_cert_path, ca_key_path, listen_addr, upstream_url):
    cfg = {
        "listen_addr": listen_addr,
        "ca_cert_path": str(ca_cert_path),
        "ca_key_path": str(ca_key_path),
        "upstream_url": upstream_url,
        "hosts": TARGET_HOSTS,
        "routes": {
            "l14-prod-sg-patch-sirius.bilibiligame.net": "http://127.0.0.1:5081",
        },
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
        "servers": [
            {
                "id": 1,
                "name": server_name,
                "cdn_root": base_cfg.get("cdn_root", ""),
                "api_server_root": base_cfg.get("api_server_root", ""),
                "chat_server_root": base_cfg.get("chat_server_root", ""),
                "at_server_root": base_cfg.get("at_server_root", ""),
                "live_server": base_cfg.get("live_server", ""),
                "area_id": base_cfg.get("area_id", ""),
            },
        ],
    }
    out = GAME_DIR / "config.json"
    out.write_text(json.dumps(cfg, indent=2))
    print(f"wrote {out}")


def main():
    base_cfg = load_base_config()
    game_port = base_cfg["game_server_port"]
    mitm_port = base_cfg["mitm_port"]
    server_name = base_cfg["server_name"].strip()

    ca_cert_path, ca_key_path = generate_ca()
    game_cert_path, game_key_path = generate_game_tls_cert()
    write_mitm_config(
        ca_cert_path,
        ca_key_path,
        listen_addr=f":{mitm_port}",
        upstream_url=f"https://127.0.0.1:{game_port}",
    )
    write_game_config(
        listen_addr=f":{game_port}",
        tls_cert_path=game_cert_path,
        tls_key_path=game_key_path,
        server_name=server_name,
    )
    print(
        "\nNext you'll just:\n"
        "  1. Install mitm/ca/ca.crt on the client device, or else.\n"
        f"  2. Set the device's HTTP(S) proxy to this machine's IP, port {mitm_port}.\n"
        "  3. run this : 'go run ./game ./game/config.json'\n"
        "  4. run this : 'go run ./mitm ./mitm/config.json'\n"
    )


if __name__ == "__main__":
    main()
