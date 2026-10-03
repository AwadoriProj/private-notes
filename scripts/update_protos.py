from __future__ import annotations

import io
import json
import shutil
import urllib.error
import urllib.request
import zipfile
from pathlib import Path, PurePosixPath

ROOT = Path(__file__).resolve().parent.parent
REPOSITORY = "awadoriproj/awadori-app-protos"
ARCHIVE_URL = f"https://codeload.github.com/{REPOSITORY}/zip/refs/heads/main"
TARGET = ROOT / "game" / "proto"
STAGING = ROOT / "game" / "proto.update.tmp"
BACKUP = ROOT / "game" / "proto.update.old"
MAX_ARCHIVE_SIZE = 20 * 1024 * 1024
REQUIRED_FILES = (
    "app/playerlogin/player_login_service.pb.go",
    "app/playerlogin/player_login_service_grpc.pb.go",
    "app/masterdata/masterdata_service.pb.go",
    "app/masterdata/masterdata_service_grpc.pb.go",
    "entity/player.pb.go",
)


def download_archive() -> bytes:
    request = urllib.request.Request(
        ARCHIVE_URL,
        headers={"User-Agent": "private-notes-proto-sync/1.0"},
    )
    try:
        response = urllib.request.urlopen(request, timeout=60)
    except urllib.error.HTTPError as error:
        raise RuntimeError(
            f"GitHub returned HTTP {error.code} for {REPOSITORY}; confirm the repository is public and its default branch is main"
        ) from error
    with response:
        length = int(response.headers.get("Content-Length", "0"))
        if length > MAX_ARCHIVE_SIZE:
            raise RuntimeError(f"proto archive exceeds {MAX_ARCHIVE_SIZE // 1024 // 1024} MiB")
        chunks = []
        size = 0
        while True:
            chunk = response.read(1024 * 1024)
            if not chunk:
                break
            size += len(chunk)
            if size > MAX_ARCHIVE_SIZE:
                raise RuntimeError(f"proto archive exceeds {MAX_ARCHIVE_SIZE // 1024 // 1024} MiB")
            chunks.append(chunk)
    return b"".join(chunks)


def extract_proto_bindings(archive: bytes) -> tuple[int, str]:
    if STAGING.exists():
        shutil.rmtree(STAGING)
    STAGING.mkdir(parents=True)
    count = 0
    version = "unknown"
    with zipfile.ZipFile(io.BytesIO(archive)) as source:
        for item in source.infolist():
            if item.is_dir():
                continue
            parts = PurePosixPath(item.filename).parts
            if ".." in parts:
                raise RuntimeError("archive contains an invalid path")
            if "global" in parts and parts[-1] == "appver.json":
                metadata = json.loads(source.read(item).decode("utf-8-sig"))
                version = str(metadata.get("version_name", version))
            if "proto" not in parts:
                continue
            index = parts.index("proto")
            relative = parts[index + 1 :]
            if not relative or not relative[-1].endswith(".pb.go"):
                continue
            destination = STAGING.joinpath(*relative)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(source.read(item))
            count += 1
    missing = [name for name in REQUIRED_FILES if not (STAGING / name).is_file()]
    if missing:
        raise RuntimeError("proto archive is missing required generated files: " + ", ".join(missing))
    return count, version


def replace_bindings() -> None:
    if BACKUP.exists():
        shutil.rmtree(BACKUP)
    if TARGET.exists():
        TARGET.replace(BACKUP)
    try:
        STAGING.replace(TARGET)
    except Exception:
        if BACKUP.exists() and not TARGET.exists():
            BACKUP.replace(TARGET)
        raise
    if BACKUP.exists():
        shutil.rmtree(BACKUP)


def main() -> None:
    archive = download_archive()
    count, version = extract_proto_bindings(archive)
    replace_bindings()
    print(f"updated {count} generated protobuf files from {REPOSITORY} (Our Notes {version})")


if __name__ == "__main__":
    main()
