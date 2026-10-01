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
    }
    out = ROOT / "mitm" / "config.json"
    out.write_text(json.dumps(cfg, indent=2))
    print(f"wrote {out}")


def write_game_config(listen_addr, tls_cert_path, tls_key_path):
    cfg = {
        "listen_addr": listen_addr,
        "tls_cert": str(tls_cert_path),
        "tls_key": str(tls_key_path),
        "rsa_key_path": str(GAME_DIR / "login_rsa.pem"),
        "servers": [
            {"id": 1, "name": "private-notes"},
        ],
    }
    out = GAME_DIR / "config.json"
    out.write_text(json.dumps(cfg, indent=2))
    print(f"wrote {out}")


def main():
    ca_cert_path, ca_key_path = generate_ca()
    game_cert_path, game_key_path = generate_game_tls_cert()
    write_mitm_config(ca_cert_path, ca_key_path, listen_addr=":8443", upstream_url="https://127.0.0.1:9443")
    write_game_config(listen_addr=":9443", tls_cert_path=game_cert_path, tls_key_path=game_key_path)
    print(
        "\nNext you'll just:\n"
        "  1. Install mitm/ca/ca.crt on the client device, or else.\n"
        "  2. Set the device's HTTP(S) proxy to this machine's IP, port 8443.\n"
        "  3. run this : 'go run ./game ./game/config.json'\n"
        "  4. run this : 'go run ./mitm ./mitm/config.json'\n"
    )


if __name__ == "__main__":
    main()