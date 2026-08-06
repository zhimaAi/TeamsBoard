#!/usr/bin/env python3
"""GoTeams database inspection and controlled query client."""

from __future__ import annotations

import argparse
import ipaddress
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

IDENTIFIER = re.compile(r"^[A-Za-z_][A-Za-z0-9_$]*$")
SAFE_PROFILE_FIELDS = {
    "id", "name", "db_type", "host", "port", "database_name", "username", "ssh_profile_id"
}


def ensure_loopback(base_url: str) -> str:
    base_url = base_url.rstrip("/")
    parsed = urllib.parse.urlparse(base_url)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise SystemExit("base URL must be an absolute http(s) URL")
    try:
        loopback = ipaddress.ip_address(parsed.hostname).is_loopback
    except ValueError:
        loopback = parsed.hostname.lower() == "localhost"
    if not loopback:
        raise SystemExit("base URL must point to localhost/loopback")
    return base_url


def checked_identifier(value: str) -> str:
    if not IDENTIFIER.fullmatch(value):
        raise SystemExit(f"unsafe SQL identifier: {value!r}")
    return value


class Client:
    def __init__(self, base_url: str, token: str, timeout: float) -> None:
        if not token:
            raise SystemExit("GOTEAMS_LOCAL_CAPABILITY_TOKEN is required")
        self.base_url = ensure_loopback(base_url)
        self.token = token
        self.timeout = timeout

    def request(
        self,
        method: str,
        path: str,
        *,
        query: dict[str, Any] | None = None,
        body: dict[str, Any] | None = None,
    ) -> Any:
        url = self.base_url + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        data = None if body is None else json.dumps(body, ensure_ascii=False).encode("utf-8")
        request = urllib.request.Request(
            url,
            data=data,
            method=method,
            headers={
                "Accept": "application/json",
                "Content-Type": "application/json",
                "X-GoTeams-Local-Token": self.token,
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                raw = response.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            raw = exc.read().decode("utf-8", errors="replace")
            try:
                message = json.loads(raw).get("error", raw)
            except json.JSONDecodeError:
                message = raw
            raise SystemExit(f"HTTP {exc.code}: {message}") from exc
        except urllib.error.URLError as exc:
            raise SystemExit(f"request failed: {exc.reason}") from exc
        return json.loads(raw) if raw else {}

    def profiles(self) -> list[dict[str, Any]]:
        return self.request(
            "GET",
            "/api/local/config/database-profiles",
            query={"page": 1, "page_size": 200},
        ).get("items", [])

    def profile(self, profile_id: int) -> dict[str, Any]:
        for profile in self.profiles():
            if int(profile.get("id", 0)) == profile_id:
                return profile
        raise SystemExit(f"database profile {profile_id} not found")

    def execute(self, profile_id: int, sql: str, confirmed: bool = False) -> Any:
        return self.request(
            "POST",
            "/api/local/tools/db/execute",
            body={
                "database_profile_id": profile_id,
                "sql": sql,
                "confirmed": confirmed,
            },
        )


def sql_from_args(args: argparse.Namespace) -> str:
    if args.sql:
        return args.sql.strip()
    with open(args.sql_file, "r", encoding="utf-8") as handle:
        return handle.read().strip()


def add_sql_source(parser: argparse.ArgumentParser) -> None:
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--sql")
    source.add_argument("--sql-file")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Inspect MySQL/PostgreSQL profiles, tables, structures and data through GoTeams."
    )
    parser.add_argument("--base-url", default=os.environ.get("GOTEAMS_LOCAL_BASE_URL", ""))
    parser.add_argument("--token", default=os.environ.get("GOTEAMS_LOCAL_CAPABILITY_TOKEN", ""))
    parser.add_argument("--timeout", type=float, default=30)
    commands = parser.add_subparsers(dest="command", required=True)

    commands.add_parser("profiles", help="list configured database profiles")

    tables = commands.add_parser("tables", help="list tables")
    tables.add_argument("--database-profile-id", type=int, required=True)
    tables.add_argument("--schema", default="public")

    structure = commands.add_parser("structure", help="show a table's columns")
    structure.add_argument("--database-profile-id", type=int, required=True)
    structure.add_argument("--table", required=True)
    structure.add_argument("--schema", default="public")

    rows = commands.add_parser("rows", help="read rows from a table")
    rows.add_argument("--database-profile-id", type=int, required=True)
    rows.add_argument("--table", required=True)
    rows.add_argument("--schema", default="public")
    rows.add_argument("--limit", type=int, default=100)
    rows.add_argument("--offset", type=int, default=0)

    query = commands.add_parser("query", help="execute SELECT/SHOW/DESC/EXPLAIN")
    query.add_argument("--database-profile-id", type=int, required=True)
    add_sql_source(query)

    validate = commands.add_parser("validate", help="validate SQL without executing it")
    add_sql_source(validate)

    write = commands.add_parser("write", help="execute one INSERT or UPDATE")
    write.add_argument("--database-profile-id", type=int, required=True)
    add_sql_source(write)
    write.add_argument(
        "--confirmed",
        action="store_true",
        help="required explicit confirmation for a controlled write",
    )
    return parser


def table_sql(profile: dict[str, Any], schema: str) -> str:
    db_type = str(profile.get("db_type", "")).lower()
    if db_type == "mysql":
        return "SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'"
    schema_literal = schema.replace("'", "''")
    return (
        "SELECT table_schema, table_name FROM information_schema.tables "
        f"WHERE table_type = 'BASE TABLE' AND table_schema = '{schema_literal}' "
        "ORDER BY table_name"
    )


def structure_sql(profile: dict[str, Any], schema: str, table: str) -> str:
    db_type = str(profile.get("db_type", "")).lower()
    table = checked_identifier(table)
    if db_type == "mysql":
        return f"DESCRIBE `{table}`"
    schema_literal = schema.replace("'", "''")
    table_literal = table.replace("'", "''")
    return (
        "SELECT ordinal_position, column_name, data_type, is_nullable, column_default "
        "FROM information_schema.columns "
        f"WHERE table_schema = '{schema_literal}' AND table_name = '{table_literal}' "
        "ORDER BY ordinal_position"
    )


def rows_sql(profile: dict[str, Any], schema: str, table: str, limit: int, offset: int) -> str:
    if limit < 1 or limit > 1000 or offset < 0:
        raise SystemExit("limit must be 1..1000 and offset must be >= 0")
    db_type = str(profile.get("db_type", "")).lower()
    table = checked_identifier(table)
    if db_type == "mysql":
        qualified = f"`{table}`"
    else:
        schema = checked_identifier(schema)
        qualified = f'"{schema}"."{table}"'
    return f"SELECT * FROM {qualified} LIMIT {limit} OFFSET {offset}"


def main() -> None:
    args = build_parser().parse_args()
    if not args.base_url:
        raise SystemExit("GOTEAMS_LOCAL_BASE_URL is required")
    client = Client(args.base_url, args.token, args.timeout)

    if args.command == "profiles":
        result: Any = {
            "items": [
                {key: value for key, value in profile.items() if key in SAFE_PROFILE_FIELDS}
                for profile in client.profiles()
            ]
        }
    elif args.command == "validate":
        result = client.request(
            "POST", "/api/local/tools/db/validate", body={"sql": sql_from_args(args)}
        )
    else:
        profile_id = args.database_profile_id
        profile = client.profile(profile_id)
        if args.command == "tables":
            result = client.execute(profile_id, table_sql(profile, args.schema))
        elif args.command == "structure":
            result = client.execute(
                profile_id, structure_sql(profile, args.schema, args.table)
            )
        elif args.command == "rows":
            result = client.execute(
                profile_id,
                rows_sql(profile, args.schema, args.table, args.limit, args.offset),
            )
        elif args.command == "query":
            result = client.execute(profile_id, sql_from_args(args))
        else:
            if not args.confirmed:
                raise SystemExit("write requires --confirmed")
            result = client.execute(profile_id, sql_from_args(args), confirmed=True)
    json.dump(result, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
