import importlib.util
import io
import pathlib
import unittest
from unittest import mock

SCRIPT = pathlib.Path(__file__).with_name("monitor-production-logs.py")
SPEC = importlib.util.spec_from_file_location("monitor_production_logs", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
monitor = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(monitor)

ALLOWED_ROUTES = frozenset({"/api/health", "/api/ai/chat", "unmatched"})
ALLOWED_EVENTS = frozenset({"http.request.completed", "studio.execution.failed"})


class MonitorProductionLogsTest(unittest.TestCase):
    def test_http_requests_include_explicit_user_agent(self):
        with mock.patch.object(monitor.urllib.request, "urlopen", return_value=io.BytesIO(b"{}")) as urlopen:
            monitor.request_json("https://example.com/test")
        request = urlopen.call_args.args[0]
        self.assertEqual(request.get_header("User-agent"), "portfolio-production-monitor/1.0")

    def test_parse_nested_docker_json(self):
        record = monitor.parse_log_line('{"log":"{\\"event\\":\\"http.request.completed\\",\\"status\\":500}\\n"}')
        self.assertEqual(record["status"], 500)

    def test_aggregate_includes_5xx_and_error_events_only(self):
        summary = monitor.aggregate_incidents(
            [
                {"event": "http.request.completed", "status": 200, "route": "/api/health"},
                {"event": "http.request.completed", "status": 500, "route": "/api/ai/chat"},
                {"event": "studio.execution.failed", "level": "error", "error_type": "deadline"},
            ],
            ALLOWED_ROUTES,
            ALLOWED_EVENTS,
        )
        self.assertEqual(summary["count"], 2)
        self.assertEqual(summary["statuses"], [("500", 1)])
        self.assertEqual(summary["routes"], [("/api/ai/chat", 1)])
        self.assertIn(("studio.execution.failed", 1), summary["events"])
        self.assertEqual(summary["error_types"], [(monitor.ERROR_TYPE_PRESENT, 1)])

    def test_aggregate_redacts_untrusted_dimensions(self):
        sentinel = "ghp_supersecrettoken123"
        summary = monitor.aggregate_incidents(
            [{"status": 500, "route": "/api/users/customer123", "event": sentinel, "error_type": sentinel}],
            ALLOWED_ROUTES,
            ALLOWED_EVENTS,
        )
        encoded = str(summary)
        self.assertNotIn("customer123", encoded)
        self.assertNotIn("supersecrettoken123", encoded)
        self.assertEqual(summary["routes"], [(monitor.REDACTED, 1)])
        self.assertEqual(summary["events"], [(monitor.REDACTED, 1)])
        self.assertEqual(summary["error_types"], [(monitor.ERROR_TYPE_PRESENT, 1)])

    def test_repo_allowlists_include_declared_routes_and_events(self):
        repo_root = pathlib.Path(__file__).resolve().parents[2]
        routes, events = monitor.load_dimension_allowlists(repo_root)
        self.assertIn("/api/health", routes)
        self.assertIn("http.request.completed", events)
        self.assertIn("studio.execution.enqueue_failed", events)

    def test_notification_lifecycle_and_reminder(self):
        summary = {
            "count": 1,
            "statuses": [("500", 1)],
            "routes": [("/api/chat", 1)],
            "events": [("http.request.completed", 1)],
            "error_types": [],
        }
        self.assertEqual(monitor.notification_kind(summary, {}, 1000, 1800), "firing")
        active = {
            "active": True,
            "last_alert_at": 900,
            "last_fingerprint": monitor.fingerprint(summary),
        }
        self.assertEqual(monitor.notification_kind(summary, active, 1000, 1800), "none")
        self.assertEqual(monitor.notification_kind(summary, active, 2800, 1800), "reminder")
        clean = {"count": 0, "statuses": [], "routes": [], "events": [], "error_types": []}
        self.assertEqual(monitor.notification_kind(clean, active, 1000, 1800), "resolved")

    def test_new_incident_signature_notifies_while_active(self):
        first = {"count": 1, "statuses": [("500", 1)], "routes": [("/api/a", 1)], "events": [], "error_types": []}
        changed = {"count": 1, "statuses": [("500", 1)], "routes": [("/api/b", 1)], "events": [], "error_types": []}
        state = {"active": True, "last_alert_at": 999, "last_fingerprint": monitor.fingerprint(first)}
        self.assertEqual(monitor.notification_kind(changed, state, 1000, 1800), "changed")

    def test_ai_malformed_response_falls_back_without_raising(self):
        with mock.patch.object(monitor, "request_json", return_value=[]):
            result = monitor.safe_ai_summary({"count": 1}, "https://example.com/ai")
        self.assertIn("AI summary", result)

    def test_loki_full_page_splits_interval_instead_of_truncating(self):
        left = {"status": 500, "route": "/api/left"}
        right = {"status": 503, "route": "/api/right"}
        with mock.patch.object(
            monitor,
            "query_loki_page",
            side_effect=[([], monitor.MAX_LINES), ([left], 1), ([right], 1)],
        ) as page:
            records = monitor.query_loki("https://logs.example/loki/api/v1/push", "user", "token", 0, 100)
        self.assertEqual(records, [left, right])
        self.assertEqual(page.call_count, 3)

    def test_loki_unsplittable_full_page_fails_closed(self):
        with mock.patch.object(monitor, "query_loki_page", return_value=([], monitor.MAX_LINES)):
            with self.assertRaisesRegex(RuntimeError, "truncated"):
                monitor.query_loki("https://logs.example/loki/api/v1/push", "user", "token", 1, 1)

    def test_loki_malformed_stream_fails_closed(self):
        malformed = {"status": "success", "data": {"result": ["not-a-stream"]}}
        with mock.patch.object(monitor, "request_json", return_value=malformed):
            with self.assertRaisesRegex(RuntimeError, "stream is malformed"):
                monitor.query_loki_page("https://logs.example/query_range", "auth", 0, 1)

    def test_loki_missing_result_and_unparseable_line_fail_closed(self):
        missing = {"status": "success", "data": {}}
        with mock.patch.object(monitor, "request_json", return_value=missing):
            with self.assertRaisesRegex(RuntimeError, "result is missing"):
                monitor.query_loki_page("https://logs.example/query_range", "auth", 0, 1)
        unparseable = {"status": "success", "data": {"result": [{"values": [["1", "plain text"]]}]}}
        with mock.patch.object(monitor, "request_json", return_value=unparseable):
            with self.assertRaisesRegex(RuntimeError, "unparseable"):
                monitor.query_loki_page("https://logs.example/query_range", "auth", 0, 1)

    def test_fingerprint_includes_dimensions_outside_top_ten(self):
        allowed_routes = frozenset(f"/api/r{i}" for i in range(12))
        records = []
        for index in range(11):
            records.extend({"status": 500, "route": f"/api/r{index}"} for _ in range(20 - index))
        first = monitor.aggregate_incidents(records, allowed_routes, frozenset())
        second = monitor.aggregate_incidents(
            records + [{"status": 500, "route": "/api/r11"}], allowed_routes, frozenset()
        )
        self.assertEqual(first["routes"], second["routes"])
        self.assertNotEqual(monitor.fingerprint(first), monitor.fingerprint(second))
        self.assertNotIn("_fingerprint", monitor.public_summary(second))

    def test_monitor_failure_message_does_not_expose_error_text(self):
        message = monitor.monitor_failure_message(RuntimeError("token=secret-value"))
        self.assertIn("RuntimeError", message)
        self.assertNotIn("secret-value", message)

    def test_discord_message_contains_only_aggregates(self):
        summary = {
            "count": 1,
            "statuses": [("500", 1)],
            "routes": [("/api/chat", 1)],
            "events": [("http.request.completed", 1)],
            "error_types": [],
        }
        message = monitor.discord_message("firing", summary, "ตรวจ route และ dependency", 300)
        self.assertIn("Incidents: **1**", message)
        self.assertIn("AI summary", message)
        self.assertLessEqual(len(message), 1950)

    def test_synthetic_alert_is_clearly_labeled(self):
        summary = {
            "count": 1,
            "statuses": [["500", 1]],
            "routes": [["/synthetic-monitor-test", 1]],
            "events": [["monitor.synthetic_test", 1]],
            "error_types": [],
            "synthetic_test": True,
        }
        message = monitor.discord_message("firing", summary, "ทดสอบระบบ", 300)
        self.assertIn("monitor test alert", message)
        self.assertNotIn("production alert", message)


if __name__ == "__main__":
    unittest.main()
