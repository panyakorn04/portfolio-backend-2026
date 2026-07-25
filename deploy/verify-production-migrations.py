#!/usr/bin/env python3
"""Verify the production Supabase migration ledger without exposing credentials."""

from __future__ import annotations

import argparse
import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ENTRY = re.compile(r"^([0-9a-f]{64})  (([0-9]{4})_[A-Za-z0-9_.-]+\.sql)$")


def load_manifest(path: Path) -> list[dict[str, str]]:
    rows: list[dict[str, str]] = []
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        match = ENTRY.fullmatch(line)
        if not match:
            raise ValueError(f"malformed manifest line {number}")
        checksum, filename, version = match.groups()
        rows.append({"version": version, "filename": filename, "checksum": checksum})
    return rows


def load_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
            value = value[1:-1]
        values[key.strip()] = value
    return values


def fetch_ledger(base_url: str, service_key: str) -> list[dict[str, str]]:
    query = urllib.parse.urlencode({"select": "version,filename,checksum", "order": "version.asc"})
    url = base_url.rstrip("/") + "/rest/v1/PortfolioMigration?" + query
    request = urllib.request.Request(
        url,
        headers={"apikey": service_key, "Authorization": f"Bearer {service_key}", "Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            payload = json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"ledger query failed with HTTP {error.code}") from error
    except urllib.error.URLError as error:
        raise RuntimeError(f"ledger query failed: {type(error.reason).__name__}") from error
    if not isinstance(payload, list):
        raise ValueError("ledger response must be an array")
    rows: list[dict[str, str]] = []
    for item in payload:
        if not isinstance(item, dict) or not all(isinstance(item.get(key), str) for key in ("version", "filename", "checksum")):
            raise ValueError("ledger response contains an invalid row")
        rows.append({key: item[key] for key in ("version", "filename", "checksum")})
    return rows


def compare(expected: list[dict[str, str]], actual: list[dict[str, str]], mode: str) -> None:
    versions = [row["version"] for row in actual]
    if versions != sorted(versions) or len(versions) != len(set(versions)):
        raise ValueError("production ledger is unordered or contains duplicate versions")
    expected_by_version = {row["version"]: row for row in expected}
    actual_by_version = {row["version"]: row for row in actual}
    for version, expected_row in expected_by_version.items():
        actual_row = actual_by_version.get(version)
        if actual_row is None:
            raise ValueError(f"production migration missing: {version}")
        if actual_row != expected_row:
            raise ValueError(f"production migration mismatch: {version}")
    if mode == "forward":
        unexpected = sorted(set(actual_by_version) - set(expected_by_version))
        if unexpected:
            raise ValueError(f"unexpected production migrations: {unexpected}")
        if len(actual) != len(expected):
            raise ValueError("production ledger does not exactly match the release")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--env-file", default="/opt/apps/.env")
    parser.add_argument("--mode", choices=("forward", "rollback"), default="forward")
    args = parser.parse_args()
    try:
        expected = load_manifest(Path(args.manifest))
        environment = load_env(Path(args.env_file))
        base_url = (environment.get("SUPABASE_URL") or environment.get("NEXT_PUBLIC_SUPABASE_URL") or "").strip()
        service_key = environment.get("SUPABASE_SERVICE_ROLE_KEY", "").strip()
        if not base_url or not service_key:
            raise ValueError("SUPABASE_URL/NEXT_PUBLIC_SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY are required")
        actual = fetch_ledger(base_url, service_key)
        compare(expected, actual, args.mode)
        print(f"production migration ledger verified: mode={args.mode} versions={len(expected)}")
        return 0
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError) as error:
        print(f"production-migrations: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
