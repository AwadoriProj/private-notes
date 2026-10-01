import argparse
import getpass
import json
import os
import platform
import secrets
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SCHEMA = ROOT / "scripts" / "setup_db.sql"
IS_WINDOWS = platform.system() == "Windows"


def run_psql(args, sql=None, sql_file=None, database="postgres", check=True, password=None, prefix=None):
    cmd = [*(prefix or []), "psql", *args, "-d", database, "-v", "ON_ERROR_STOP=1"]
    if sql_file:
        cmd += ["-f", str(sql_file)]
    elif sql:
        cmd += ["-c", sql]
    env = os.environ.copy()
    if password is not None:
        env["PGPASSWORD"] = password
    result = subprocess.run(cmd, capture_output=True, text=True, env=env)
    if result.stdout.strip():
        print(result.stdout.strip())
    if result.returncode != 0:
        print(result.stderr.strip(), file=sys.stderr)
        if check:
            raise SystemExit(result.returncode)
    return result


def create_role_and_db(admin_args, role, password, dbname, prefix, admin_password):
    exists = run_psql(
        admin_args,
        sql=f"SELECT 1 FROM pg_roles WHERE rolname='{role}'",
        check=False,
        prefix=prefix,
        password=admin_password,
    )
    if "1 row" in exists.stdout:
        print(f"role {role} already exists, skipping CREATE ROLE")
    else:
        run_psql(admin_args, sql=f"CREATE ROLE {role} LOGIN PASSWORD '{password}';", prefix=prefix, password=admin_password)

    exists_db = run_psql(
        admin_args,
        sql=f"SELECT 1 FROM pg_database WHERE datname='{dbname}'",
        check=False,
        prefix=prefix,
        password=admin_password,
    )
    if "1 row" in exists_db.stdout:
        print(f"database {dbname} already exists, skipping CREATE DATABASE")
    else:
        run_psql(admin_args, sql=f"CREATE DATABASE {dbname} OWNER {role};", prefix=prefix, password=admin_password)


def main():
    p = argparse.ArgumentParser(description="Set up the private-notes Postgres database via psql.")
    p.add_argument("--host", default="127.0.0.1")
    p.add_argument("--port", default="5432")
    p.add_argument("--admin-user", default="postgres", help="role used to run CREATE ROLE/DATABASE")
    p.add_argument("--admin-password", default=None, help="password for --admin-user; prompted if needed and omitted")
    p.add_argument("--role", default="private_notes")
    p.add_argument("--dbname", default="private_notes")
    p.add_argument("--password", default=None, help="generated if omitted")
    p.add_argument("--skip-create", action="store_true", help="assume role/db already exist, only apply schema")
    p.add_argument(
        "--no-sudo",
        action="store_true",
        help="don't prefix admin commands with `sudo -u postgres` (always off on Windows, where that concept doesn't exist)",
    )
    p.add_argument("--write-config", default=str(ROOT / "game" / "db.json"))
    args = p.parse_args()

    password = args.password or secrets.token_hex(16)
    use_sudo = not args.no_sudo and not IS_WINDOWS
    admin_prefix = ["sudo", "-u", "postgres"] if use_sudo else []

    if use_sudo:
        admin_args = ["-U", args.admin_user]
        admin_password = None
    else:
        admin_args = ["-h", args.host, "-p", args.port, "-U", args.admin_user]
        admin_password = args.admin_password
        if admin_password is None and not args.skip_create:
            admin_password = getpass.getpass(f"password for postgres role '{args.admin_user}': ")

    app_args = ["-h", args.host, "-p", args.port, "-U", args.role]

    if not args.skip_create:
        create_role_and_db(admin_args, args.role, password, args.dbname, admin_prefix, admin_password)
    else:
        print("skip-create set, not touching role/database, --password must match the existing role")

    run_psql(app_args, sql_file=SCHEMA, database=args.dbname, password=password)

    dsn = f"postgres://{args.role}:{password}@{args.host}:{args.port}/{args.dbname}?sslmode=disable"
    Path(args.write_config).write_text(json.dumps({"dsn": dsn}, indent=2))
    print(f"\nwrote {args.write_config}")
    print("merge its \"dsn\" value into game/config.json as \"db_dsn\" (or point your deploy's secret store at it)")


if __name__ == "__main__":
    main()