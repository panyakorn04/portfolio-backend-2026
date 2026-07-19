#!/usr/bin/env python3
"""Query production Loki logs, summarize incidents, and notify Discord."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import Counter
from pathlib import Path
from typing import Any

STREAM_SELECTOR = '{application="portfolio-api",environment="production"}'
DEFAULT_LOOKBACK_SECONDS = 300
DEFAULT_REMINDER_SECONDS = 1800
MAX_LINES = 5000
MAX_QUERY_SPLITS = 20
MAX_QUERY_PAGES = 128
REDACTED = "redacted"
ERROR_TYPE_PRESENT = "present"
EVENT_LITERAL = re.compile(r'^[a-z][a-z0-9_.-]{0,127}$')


def env(name: str, *, required: bool = True, default: str = "") -> str:
    value = os.environ.get(name, "").strip() or default
    if required and not value:
        raise RuntimeError(f"{name} is required")
    return value


def request_json(
    url: str,
    *,
    method: str = "GET",
    headers: dict[str, str] | None = None,
    body: dict[str, Any] | None = None,
    timeout: int = 30,
) -> Any:
    data = None
    request_headers = {
        "Accept": "application/json",
        "User-Agent": "portfolio-production-monitor/1.0",
        **(headers or {}),
    }
    if body is not None:
        data = json.dumps(body, separators=(",", ":")).encode()
        request_headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=request_headers, method=method)
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return json.load(response)


def parse_log_line(raw: str) -> dict[str, Any] | None:
    try:
        record = json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        return None
    if not isinstance(record, dict):
        return None

    nested = record.get("log")
    if isinstance(nested, str):
        try:
            parsed_nested = json.loads(nested.strip())
        except json.JSONDecodeError:
            parsed_nested = None
        if isinstance(parsed_nested, dict):
            record = parsed_nested
    return record


def safe_status(value: Any) -> int | None:
    try:
        status = int(value)
    except (TypeError, ValueError):
        return None
    return status if 100 <= status <= 599 else None


def allowlisted_dimension(value: Any, allowed: frozenset[str]) -> str:
    text = str(value).strip()
    if not text:
        return ""
    return text if text in allowed else REDACTED


def load_dimension_allowlists(repo_root: Path) -> tuple[frozenset[str], frozenset[str]]:
    api_path = repo_root / "portfolio.api"
    try:
        api_source = api_path.read_text()
    except OSError as error:
        raise RuntimeError("Unable to load the trusted route allowlist source") from error
    routes = {
        match.group(1)
        for match in re.finditer(r"^\s*(?:get|post|put|patch|delete)\s+(/\S+)", api_source, re.MULTILINE)
    }
    if not routes:
        raise RuntimeError("Trusted route allowlist is empty")
    routes.add("unmatched")

    events: set[str] = set()
    try:
        go_files = list((repo_root / "internal").rglob("*.go"))
        for path in go_files:
            source = path.read_text()
            events.update(re.findall(r'logx\.Field\("event",\s*"([a-zA-Z0-9_.-]+)"\)', source))
            events.update(
                re.findall(
                    r'observability\.(?:Error|ErrorType)\([^,]+,\s*"([a-zA-Z0-9_.-]+)"\s*,',
                    source,
                )
            )
    except OSError as error:
        raise RuntimeError("Unable to load the trusted event allowlist source") from error
    events = {event for event in events if EVENT_LITERAL.fullmatch(event)}
    if not events:
        raise RuntimeError("Trusted event allowlist is empty")
    return frozenset(routes), frozenset(events)


def is_incident(record: dict[str, Any], allowed_events: frozenset[str]) -> bool:
    status = safe_status(record.get("status"))
    level = str(record.get("level", "")).lower()
    event = allowlisted_dimension(record.get("event", ""), allowed_events).lower()
    return bool(
        (status is not None and status >= 500)
        or level in {"error", "fatal", "panic"}
        or event.endswith(".failed")
        or event.endswith("_failed")
    )


def aggregate_incidents(
    records: list[dict[str, Any]],
    allowed_routes: frozenset[str],
    allowed_events: frozenset[str],
) -> dict[str, Any]:
    incidents = [record for record in records if is_incident(record, allowed_events)]
    statuses: Counter[str] = Counter()
    routes: Counter[str] = Counter()
    events: Counter[str] = Counter()
    error_types: Counter[str] = Counter()

    for record in incidents:
        status = safe_status(record.get("status"))
        if status is not None and status >= 500:
            statuses[str(status)] += 1
        route = allowlisted_dimension(record.get("route", ""), allowed_routes)
        if route:
            routes[route] += 1
        event = allowlisted_dimension(record.get("event", ""), allowed_events)
        if event:
            events[event] += 1
        if str(record.get("error_type", "")).strip():
            error_types[ERROR_TYPE_PRESENT] += 1

    signature_dimensions = {
        "statuses": sorted(statuses),
        "routes": sorted(routes),
        "events": sorted(events),
        "error_types": sorted(error_types),
    }
    signature = hashlib.sha256(
        json.dumps(signature_dimensions, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()[:16]

    return {
        "count": len(incidents),
        "statuses": statuses.most_common(10),
        "routes": routes.most_common(10),
        "events": events.most_common(10),
        "error_types": error_types.most_common(10),
        "_fingerprint": signature,
    }


def fingerprint(summary: dict[str, Any]) -> str:
    stored = summary.get("_fingerprint")
    if isinstance(stored, str) and re.fullmatch(r"[0-9a-f]{16}", stored):
        return stored
    dimensions = {
        key: sorted(str(item[0]) for item in summary.get(key, []))
        for key in ("statuses", "routes", "events", "error_types")
    }
    canonical = json.dumps(dimensions, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(canonical.encode()).hexdigest()[:16]


def public_summary(summary: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in summary.items() if not key.startswith("_")}


def load_state(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text())
    except (FileNotFoundError, json.JSONDecodeError, OSError):
        return {}
    return value if isinstance(value, dict) else {}


def notification_kind(
    summary: dict[str, Any], state: dict[str, Any], now: int, reminder_seconds: int
) -> str:
    active = bool(state.get("active"))
    count = int(summary.get("count", 0))
    if count > 0 and not active:
        return "firing"
    if count > 0 and active and fingerprint(summary) != str(state.get("last_fingerprint", "")):
        return "changed"
    if count > 0 and active and now - int(state.get("last_alert_at", 0)) >= reminder_seconds:
        return "reminder"
    if count == 0 and active:
        return "resolved"
    return "none"


def format_pairs(values: list[list[Any]] | list[tuple[Any, Any]]) -> str:
    if not values:
        return "none"
    return ", ".join(f"{key} ({count})" for key, count in values[:5])


def ai_summary(summary: dict[str, Any], ai_url: str) -> str:
    prompt = (
        "คุณเป็น production incident analyst สรุปข้อมูล aggregate ต่อไปนี้เป็นภาษาไทยไม่เกิน 500 ตัวอักษร "
        "ระบุผลกระทบที่เป็นไปได้ จุดที่ควรตรวจ และห้ามเดาสาเหตุที่ไม่มีหลักฐาน "
        "ข้อมูลไม่มี raw logs หรือข้อมูลผู้ใช้:\n"
        + json.dumps(public_summary(summary), ensure_ascii=False, separators=(",", ":"))
    )
    response = request_json(ai_url, method="POST", body={"prompt": prompt}, timeout=120)
    if not isinstance(response, dict) or not isinstance(response.get("data"), dict):
        raise ValueError("AI response must contain an object data field")
    text = response["data"].get("response", "")
    if not isinstance(text, str) or not text.strip():
        raise ValueError("AI response text is missing")
    return text.strip()[:700]


def safe_ai_summary(summary: dict[str, Any], ai_url: str) -> str:
    try:
        return ai_summary(summary, ai_url)
    except Exception as error:  # AI enrichment must never block the deterministic alert.
        print(f"AI summary unavailable: {type(error).__name__}", file=sys.stderr)
        return "AI summary ใช้งานไม่ได้ชั่วคราว กรุณาตรวจ Grafana ตาม aggregate ด้านบน"


def discord_message(kind: str, summary: dict[str, Any], analysis: str, window_seconds: int) -> str:
    if kind == "resolved":
        return "✅ **Portfolio API incident resolved**\nไม่พบ HTTP 5xx หรือ error events ในช่วงตรวจล่าสุด"

    if summary.get("synthetic_test"):
        heading = "🧪 **Portfolio API monitor test alert**"
    else:
        headings = {
            "firing": "🚨 **Portfolio API production alert**",
            "changed": "🆕 **Portfolio API new incident signature**",
            "reminder": "⚠️ **Portfolio API incident reminder**",
        }
        heading = headings.get(kind, headings["reminder"])
    lines = [
        heading,
        f"Window: last {window_seconds // 60} minutes | Incidents: **{summary['count']}**",
        f"Statuses: {format_pairs(summary['statuses'])}",
        f"Routes: {format_pairs(summary['routes'])}",
        f"Events: {format_pairs(summary['events'])}",
        f"Error types: {format_pairs(summary['error_types'])}",
    ]
    if analysis:
        lines.extend(["", "**AI summary (sanitized aggregate only)**", analysis])
    lines.extend(["", "Grafana query: `{application=\"portfolio-api\",environment=\"production\"} | json`"])
    return "\n".join(lines)[:1950]


def query_loki_page(
    query_url: str, auth: str, start_ns: int, end_ns: int
) -> tuple[list[dict[str, Any]], int]:
    params = urllib.parse.urlencode(
        {
            "query": STREAM_SELECTOR,
            "start": str(start_ns),
            "end": str(end_ns),
            "limit": str(MAX_LINES),
            "direction": "forward",
        }
    )
    response = request_json(f"{query_url}?{params}", headers={"Authorization": f"Basic {auth}"})
    if not isinstance(response, dict) or response.get("status") != "success":
        raise RuntimeError("Loki query did not return success")

    records: list[dict[str, Any]] = []
    raw_count = 0
    data = response.get("data")
    if not isinstance(data, dict):
        raise RuntimeError("Loki query response data is malformed")
    if "result" not in data:
        raise RuntimeError("Loki query response result is missing")
    results = data["result"]
    if not isinstance(results, list):
        raise RuntimeError("Loki query response result is malformed")
    for stream in results:
        if not isinstance(stream, dict):
            raise RuntimeError("Loki query stream is malformed")
        if "values" not in stream:
            raise RuntimeError("Loki query values are missing")
        values = stream["values"]
        if not isinstance(values, list):
            raise RuntimeError("Loki query values are malformed")
        for value in values:
            if not isinstance(value, list) or len(value) != 2:
                raise RuntimeError("Loki query log entry is malformed")
            raw_count += 1
            record = parse_log_line(value[1])
            if record is None:
                raise RuntimeError("Loki query returned an unparseable structured log entry")
            records.append(record)
    return records, raw_count


def query_loki(
    push_url: str,
    username: str,
    token: str,
    start_ns: int,
    end_ns: int,
) -> list[dict[str, Any]]:
    query_url = push_url.removesuffix("/loki/api/v1/push") + "/loki/api/v1/query_range"
    auth = base64.b64encode(f"{username}:{token}".encode()).decode()
    pending = [(start_ns, end_ns, 0)]
    all_records: list[dict[str, Any]] = []
    pages = 0
    while pending:
        if pages >= MAX_QUERY_PAGES:
            raise RuntimeError("Loki query exceeded the bounded page budget")
        interval_start, interval_end, split_depth = pending.pop()
        records, raw_count = query_loki_page(query_url, auth, interval_start, interval_end)
        pages += 1
        if raw_count < MAX_LINES:
            all_records.extend(records)
            continue
        if interval_start >= interval_end or split_depth >= MAX_QUERY_SPLITS:
            raise RuntimeError("Loki result remained truncated after bounded interval splitting")
        midpoint = interval_start + (interval_end - interval_start) // 2
        pending.append((midpoint + 1, interval_end, split_depth + 1))
        pending.append((interval_start, midpoint, split_depth + 1))
    return all_records


def send_discord(webhook_url: str, content: str) -> None:
    request_json(
        webhook_url + ("&" if "?" in webhook_url else "?") + "wait=true",
        method="POST",
        body={"content": content, "allowed_mentions": {"parse": []}},
    )


def monitor_failure_message(error: Exception) -> str:
    return (
        "🔴 **Portfolio API log monitor failure**\n"
        f"The Loki monitoring query failed safely (`{type(error).__name__}`). "
        "No incident state was changed; inspect the GitHub Actions run and Grafana availability."
    )


def main() -> int:
    push_url = env("GRAFANA_LOKI_URL")
    username = env("GRAFANA_LOKI_USERNAME")
    token = env("GRAFANA_LOKI_TOKEN")
    webhook_url = env("DISCORD_WEBHOOK_URL")
    ai_url = env("AI_LOG_SUMMARY_URL", required=False, default="https://api.panyakorn.com/api/ai/generate")
    state_path = Path(env("MONITOR_STATE_FILE", required=False, default=".monitor-state/state.json"))
    dry_run = env("MONITOR_DRY_RUN", required=False, default="false").lower() == "true"
    test_alert = env("MONITOR_TEST_ALERT", required=False, default="false").lower() == "true"
    reminder_seconds = int(env("MONITOR_REMINDER_SECONDS", required=False, default=str(DEFAULT_REMINDER_SECONDS)))

    if not push_url.startswith("https://") or not push_url.endswith("/loki/api/v1/push"):
        raise RuntimeError("GRAFANA_LOKI_URL must be an HTTPS Loki push endpoint")
    if not webhook_url.startswith("https://discord.com/api/webhooks/"):
        raise RuntimeError("DISCORD_WEBHOOK_URL must be a Discord webhook URL")

    now = int(time.time())
    state = load_state(state_path)
    previous_checked = int(state.get("last_checked_at", 0))
    start_seconds = max(previous_checked - 30, now - 900) if previous_checked else now - DEFAULT_LOOKBACK_SECONDS
    try:
        repo_root = Path(__file__).resolve().parents[2]
        allowed_routes, allowed_events = load_dimension_allowlists(repo_root)
        records = query_loki(push_url, username, token, start_seconds * 1_000_000_000, now * 1_000_000_000)
    except Exception as error:
        failure = monitor_failure_message(error)
        if dry_run:
            print(failure)
        else:
            send_discord(webhook_url, failure)
        raise RuntimeError("Loki monitoring query failed") from error
    summary = aggregate_incidents(records, allowed_routes, allowed_events)
    if test_alert:
        summary = {
            "count": 1,
            "statuses": [["500", 1]],
            "routes": [["/synthetic-monitor-test", 1]],
            "events": [["monitor.synthetic_test", 1]],
            "error_types": [],
            "synthetic_test": True,
        }
        kind = "firing"
    else:
        kind = notification_kind(summary, state, now, reminder_seconds)
    print(json.dumps({"kind": kind, "summary": public_summary(summary)}, ensure_ascii=False))

    analysis = ""
    if kind in {"firing", "changed", "reminder"}:
        analysis = safe_ai_summary(summary, ai_url)

    if kind != "none":
        message = discord_message(kind, summary, analysis, now - start_seconds)
        if dry_run:
            print(message)
        else:
            send_discord(webhook_url, message)

    if not dry_run and not test_alert:
        state_path.parent.mkdir(parents=True, exist_ok=True)
        next_state = {
            "active": summary["count"] > 0,
            "last_checked_at": now,
            "last_alert_at": now if kind in {"firing", "changed", "reminder"} else int(state.get("last_alert_at", 0)),
            "last_fingerprint": fingerprint(summary) if summary["count"] > 0 else "",
        }
        state_path.write_text(json.dumps(next_state, sort_keys=True) + "\n")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (RuntimeError, urllib.error.URLError, TimeoutError, ValueError) as error:
        print(f"Production log monitor failed: {error}", file=sys.stderr)
        raise SystemExit(1)
