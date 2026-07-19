import importlib.util
import pathlib
import unittest

SCRIPT = pathlib.Path(__file__).with_name("monitor-production-logs.py")
SPEC = importlib.util.spec_from_file_location("monitor_production_logs", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
monitor = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(monitor)


class MonitorProductionLogsTest(unittest.TestCase):
    def test_parse_nested_docker_json(self):
        record = monitor.parse_log_line('{"log":"{\\"event\\":\\"http.request.completed\\",\\"status\\":500}\\n"}')
        self.assertEqual(record["status"], 500)

    def test_aggregate_includes_5xx_and_error_events_only(self):
        summary = monitor.aggregate_incidents(
            [
                {"event": "http.request.completed", "status": 200, "route": "/api/health"},
                {"event": "http.request.completed", "status": 500, "route": "/api/chat"},
                {"event": "studio.execution.failed", "level": "error", "error_type": "deadline"},
            ]
        )
        self.assertEqual(summary["count"], 2)
        self.assertEqual(summary["statuses"], [("500", 1)])
        self.assertEqual(summary["routes"], [("/api/chat", 1)])
        self.assertIn(("studio.execution.failed", 1), summary["events"])

    def test_notification_lifecycle_and_reminder(self):
        summary = {"count": 1}
        self.assertEqual(monitor.notification_kind(summary, {}, 1000, 1800), "firing")
        active = {"active": True, "last_alert_at": 900}
        self.assertEqual(monitor.notification_kind(summary, active, 1000, 1800), "none")
        self.assertEqual(monitor.notification_kind(summary, active, 2800, 1800), "reminder")
        self.assertEqual(monitor.notification_kind({"count": 0}, active, 1000, 1800), "resolved")

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
