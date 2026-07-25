#!/usr/bin/env python3

from __future__ import annotations

import json
import subprocess
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "deploy/verify-production-migrations.py"
CHECKSUM = "a" * 64


class LedgerHandler(BaseHTTPRequestHandler):
    rows: list[dict[str, str]] = []

    def do_GET(self) -> None:
        if self.headers.get("apikey") != "super-secret-test-key":
            self.send_response(401)
            self.end_headers()
            return
        body = json.dumps(type(self).rows).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: object) -> None:
        del format, args
        return


class ProductionMigrationVerifierTests(unittest.TestCase):
    def setUp(self) -> None:
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), LedgerHandler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.tempdir = tempfile.TemporaryDirectory()
        root = Path(self.tempdir.name)
        self.manifest = root / "manifest.sha256"
        self.manifest.write_text(f"{CHECKSUM}  0001_init.sql\n", encoding="utf-8")
        self.env = root / ".env"
        self.env.write_text(
            f"NEXT_PUBLIC_SUPABASE_URL=http://127.0.0.1:{self.server.server_port}\n"
            "SUPABASE_SERVICE_ROLE_KEY=super-secret-test-key\n",
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)
        self.tempdir.cleanup()

    def run_verifier(self, mode: str = "forward") -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["python3", str(SCRIPT), "--manifest", str(self.manifest), "--env-file", str(self.env), "--mode", mode],
            text=True, capture_output=True, check=False,
        )

    def test_exact_forward_ledger_passes(self) -> None:
        LedgerHandler.rows = [{"version": "0001", "filename": "0001_init.sql", "checksum": CHECKSUM}]
        result = self.run_verifier()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("super-secret-test-key", result.stdout + result.stderr)

    def test_mismatch_fails_without_exposing_key(self) -> None:
        LedgerHandler.rows = [{"version": "0001", "filename": "0001_init.sql", "checksum": "b" * 64}]
        result = self.run_verifier()
        self.assertEqual(result.returncode, 1)
        self.assertIn("mismatch: 0001", result.stderr)
        self.assertNotIn("super-secret-test-key", result.stdout + result.stderr)

    def test_rollback_allows_additive_superset(self) -> None:
        LedgerHandler.rows = [
            {"version": "0001", "filename": "0001_init.sql", "checksum": CHECKSUM},
            {"version": "0002", "filename": "0002_next.sql", "checksum": "b" * 64},
        ]
        result = self.run_verifier("rollback")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
