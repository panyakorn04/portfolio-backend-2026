import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
MIGRATIONS = ROOT / "migrations"
SECURITY_MIGRATION = MIGRATIONS / "0018_backend_table_access_security.sql"


def quoted_names(value: str) -> set[str]:
    return set(re.findall(r'"([A-Za-z][A-Za-z0-9]*)"', value))


class BackendTableSecurityMigrationTest(unittest.TestCase):
    def test_every_application_table_is_backend_only(self):
        created_tables: set[str] = set()
        for path in sorted(MIGRATIONS.glob("*.sql")):
            created_tables.update(
                re.findall(
                    r'CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+"([^"]+)"',
                    path.read_text(),
                    re.IGNORECASE,
                )
            )

        security_sql = SECURITY_MIGRATION.read_text()
        rls_tables = set(
            re.findall(
                r'ALTER\s+TABLE\s+"([^"]+)"\s+ENABLE\s+ROW\s+LEVEL\s+SECURITY',
                security_sql,
                re.IGNORECASE,
            )
        )
        revoke_blocks = re.findall(
            r'REVOKE\s+ALL\s+ON\s+TABLE(.*?)FROM\s+PUBLIC,\s*anon,\s*authenticated\s*;',
            security_sql,
            re.IGNORECASE | re.DOTALL,
        )
        grant_blocks = re.findall(
            r'GRANT\s+ALL\s+ON\s+TABLE(.*?)TO\s+service_role\s*;',
            security_sql,
            re.IGNORECASE | re.DOTALL,
        )
        revoked_tables = set().union(*(quoted_names(block) for block in revoke_blocks))
        granted_tables = set().union(*(quoted_names(block) for block in grant_blocks))

        self.assertTrue(created_tables, "No application tables were discovered")
        self.assertEqual(created_tables - rls_tables, set(), "Tables missing RLS")
        self.assertEqual(created_tables - revoked_tables, set(), "Tables missing anon/public revoke")
        self.assertEqual(created_tables - granted_tables, set(), "Tables missing service_role grant")

    def test_future_table_default_privileges_are_backend_only(self):
        normalized = " ".join(SECURITY_MIGRATION.read_text().split()).lower()
        self.assertIn(
            "alter default privileges for role postgres in schema public revoke all on tables from public, anon, authenticated;",
            normalized,
        )
        self.assertIn(
            "alter default privileges for role postgres in schema public grant all on tables to service_role;",
            normalized,
        )


if __name__ == "__main__":
    unittest.main()
