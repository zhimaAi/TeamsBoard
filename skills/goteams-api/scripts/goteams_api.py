#!/usr/bin/env python3
"""Task-scoped GoTeams API manager client."""

from __future__ import annotations

import argparse
import ipaddress
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any


def env_int(name: str) -> int:
    value = os.environ.get(name, "").strip()
    if not value.isdigit() or int(value) <= 0:
        raise SystemExit(f"{name} is required and must be a positive integer")
    return int(value)


def json_value(value: str, expected: type) -> Any:
    try:
        parsed = json.loads(value)
    except json.JSONDecodeError as exc:
        raise argparse.ArgumentTypeError(f"invalid JSON: {exc}") from exc
    if not isinstance(parsed, expected):
        raise argparse.ArgumentTypeError(f"JSON must be {expected.__name__}")
    return parsed


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


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Manage API definitions inside the current task's GoTeams folder."
    )
    parser.add_argument("--base-url", default=os.environ.get("GOTEAMS_LOCAL_BASE_URL", ""))
    parser.add_argument("--token", default=os.environ.get("GOTEAMS_LOCAL_CAPABILITY_TOKEN", ""))
    parser.add_argument("--timeout", type=float, default=30)
    commands = parser.add_subparsers(dest="command", required=True)

    commands.add_parser("context", help="show the authorized collection and folder")
    commands.add_parser("list", help="list interfaces in the authorized folder")

    get_cmd = commands.add_parser("get", help="get one interface's details")
    get_cmd.add_argument("id", type=int)

    create = commands.add_parser("create", help="create an interface in the authorized folder")
    create.add_argument("--name", required=True)
    create.add_argument("--method", default="GET")
    create.add_argument("--url", required=True)
    create.add_argument("--description", default="")
    create.add_argument("--headers", type=lambda v: json_value(v, dict), default={})
    create.add_argument("--query", type=lambda v: json_value(v, list), default=[])
    create.add_argument("--auth", type=lambda v: json_value(v, dict), default={"type": "none"})
    create.add_argument("--body", default="")
    create.add_argument(
        "--body-type",
        choices=["none", "json", "text", "x-www-form-urlencoded", "multipart"],
        default="none",
    )
    create.add_argument("--body-form", type=lambda v: json_value(v, list), default=[])
    create.add_argument("--environment-id", type=int, default=0)

    update = commands.add_parser("update", help="modify an interface in the authorized folder")
    update.add_argument("id", type=int)
    update.add_argument(
        "--data",
        required=True,
        type=lambda v: json_value(v, dict),
        help="JSON object containing fields to change",
    )

    delete = commands.add_parser("delete", help="delete an interface in the authorized folder")
    delete.add_argument("id", type=int)

    execute = commands.add_parser("execute", help="execute a saved interface")
    execute.add_argument("id", type=int)
    execute.add_argument("--environment-id", type=int, default=0)

    history = commands.add_parser("history", help="show local execution history")
    history.add_argument("id", type=int)
    history.add_argument("--limit", type=int, default=50)
    return parser


def main() -> None:
    args = build_parser().parse_args()
    collection_id = env_int("GOTEAMS_API_COLLECTION_ID")
    folder_id = env_int("GOTEAMS_API_FOLDER_ID")
    if not args.base_url:
        raise SystemExit("GOTEAMS_LOCAL_BASE_URL is required")
    client = Client(args.base_url, args.token, args.timeout)

    if args.command == "context":
        collection = client.request("GET", "/api/local/apis/collections").get("data", [])
        folder = client.request(
            "GET", "/api/local/apis/folders", query={"collection_id": collection_id}
        ).get("data", [])
        result: Any = {
            "collection_id": collection_id,
            "folder_id": folder_id,
            "collection": collection[0] if collection else None,
            "folder": folder[0] if folder else None,
        }
    elif args.command == "list":
        result = client.request(
            "GET",
            "/api/local/apis/requests",
            query={"collection_id": collection_id, "folder_id": folder_id},
        )
    elif args.command == "get":
        result = client.request("GET", f"/api/local/apis/requests/{args.id}")
    elif args.command == "create":
        result = client.request(
            "POST",
            "/api/local/apis/requests",
            body={
                "collection_id": collection_id,
                "folder_id": folder_id,
                "name": args.name,
                "method": args.method.upper(),
                "url": args.url,
                "description": args.description,
                "headers": args.headers,
                "query": args.query,
                "auth": args.auth,
                "body": args.body,
                "body_type": args.body_type,
                "body_form": args.body_form,
                "environment_id": args.environment_id,
            },
        )
    elif args.command == "update":
        data = dict(args.data)
        for key, required in {
            "collection_id": collection_id,
            "folder_id": folder_id,
        }.items():
            if key in data and data[key] != required:
                raise SystemExit(f"{key} cannot be changed outside the task scope")
        data["collection_id"] = collection_id
        data["folder_id"] = folder_id
        result = client.request("PUT", f"/api/local/apis/requests/{args.id}", body=data)
    elif args.command == "delete":
        result = client.request("DELETE", f"/api/local/apis/requests/{args.id}")
    elif args.command == "execute":
        result = client.request(
            "POST",
            f"/api/local/apis/requests/{args.id}/execute",
            body={"environment_id": args.environment_id},
        )
    else:
        result = client.request(
            "GET",
            f"/api/local/apis/requests/{args.id}/history",
            query={"limit": args.limit},
        )
    json.dump(result, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
