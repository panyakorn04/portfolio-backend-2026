#!/usr/bin/env python3

from __future__ import annotations

import hashlib
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / ".github/scripts/migration-ledger.py"


class MigrationLedgerTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        scripts = self.root / ".github/scripts"
        scripts.mkdir(parents=True)
        shutil.copy2(SCRIPT, scripts / SCRIPT.name)
        shutil.copytree(ROOT / "migrations", self.root / "migrations")
        self.script = scripts / SCRIPT.name

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def run_script(self, *args: str, expected: int = 0) -> subprocess.CompletedProcess[str]:
        result = subprocess.run(
            ["python3", str(self.script), *args], cwd=self.root,
            text=True, capture_output=True, check=False,
        )
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        return result

    def refresh_manifest_entry(self, filename: str) -> None:
        path = self.root / "migrations" / filename
        checksum = hashlib.sha256(path.read_bytes()).hexdigest()
        manifest = self.root / "migrations/manifest.sha256"
        lines = manifest.read_text(encoding="utf-8").splitlines()
        lines = [f"{checksum}  {filename}" if line.endswith(f"  {filename}") else line for line in lines]
        manifest.write_text("\n".join(lines) + "\n", encoding="utf-8")

    def test_verify_and_bootstrap_bundle(self) -> None:
        result = self.run_script("verify")
        migration_count = len(list((self.root / "migrations").glob("[0-9][0-9][0-9][0-9]_*.sql")))
        self.assertIn(f"{migration_count} migrations", result.stdout)
        bundle = self.run_script(
            "bootstrap", "--baseline-through", "0019", "--through", "0022"
        ).stdout
        self.assertIn("BEGIN;", bundle)
        self.assertIn("pg_advisory_xact_lock", bundle)
        self.assertIn("BEGIN 0020_portfolio_chat_transactions_retention.sql", bundle)
        self.assertIn("BEGIN 0021_migration_ledger.sql", bundle)
        self.assertIn("BEGIN 0022_portfolio_chat_run_claims.sql", bundle)
        self.assertIn("('0001', '0001_init.sql'", bundle)
        self.assertIn("('0021', '0021_migration_ledger.sql'", bundle)
        self.assertIn("('0022', '0022_portfolio_chat_run_claims.sql'", bundle)

        incremental = self.run_script(
            "bootstrap", "--baseline-through", "0021", "--through", "0022"
        ).stdout
        self.assertIn("existing migration ledger does not match the reviewed baseline", incremental)
        self.assertNotIn("BEGIN 0021_migration_ledger.sql", incremental)
        self.assertIn("BEGIN 0022_portfolio_chat_run_claims.sql", incremental)
        insert_section = incremental.rsplit(
            'INSERT INTO "PortfolioMigration"', maxsplit=1
        )[1]
        self.assertNotIn("('0021', '0021_migration_ledger.sql'", insert_section)
        self.assertIn("('0022', '0022_portfolio_chat_run_claims.sql'", insert_section)

    def test_checksum_tamper_fails(self) -> None:
        path = self.root / "migrations/0001_init.sql"
        path.write_text(path.read_text(encoding="utf-8") + "\n-- tampered\n", encoding="utf-8")
        result = self.run_script("verify", expected=1)
        self.assertIn("checksum mismatch", result.stderr)

    def test_historical_change_fails_even_with_refreshed_manifest(self) -> None:
        subprocess.run(["git", "init", "-q"], cwd=self.root, check=True)
        subprocess.run(["git", "config", "user.email", "test@example.com"], cwd=self.root, check=True)
        subprocess.run(["git", "config", "user.name", "Migration Test"], cwd=self.root, check=True)
        subprocess.run(["git", "add", "migrations"], cwd=self.root, check=True)
        subprocess.run(["git", "commit", "-qm", "baseline"], cwd=self.root, check=True)
        path = self.root / "migrations/0001_init.sql"
        path.write_text(path.read_text(encoding="utf-8") + "\n-- rewritten\n", encoding="utf-8")
        self.refresh_manifest_entry("0001_init.sql")
        result = self.run_script("verify", "--base-ref", "HEAD", expected=1)
        self.assertIn("historical migration modified", result.stderr)


if __name__ == "__main__":
    unittest.main()
