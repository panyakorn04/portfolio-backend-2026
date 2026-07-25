#!/usr/bin/env bash
set -Eeuo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
    CREATE ROLE anon NOLOGIN;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
    CREATE ROLE authenticated NOLOGIN;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role') THEN
    CREATE ROLE service_role NOLOGIN BYPASSRLS;
  END IF;
END
$$;
CREATE SCHEMA IF NOT EXISTS storage;
CREATE TABLE IF NOT EXISTS storage.buckets (
  id text PRIMARY KEY,
  name text NOT NULL,
  public boolean NOT NULL DEFAULT false,
  file_size_limit bigint,
  allowed_mime_types text[]
);
SQL

bundle="$(mktemp)"
trap 'rm -f "$bundle"' EXIT
through="$(python3 -c 'from pathlib import Path; print(Path("migrations/manifest.sha256").read_text().splitlines()[-1].split()[1].split("_", 1)[0])')"
python3 .github/scripts/migration-ledger.py bootstrap \
  --baseline-through 0000 --through "$through" --output "$bundle"
echo "Applying generated clean-database migration bundle through ${through}"
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --file "$bundle"

for migration in migrations/*.sql; do
  echo "Reapplying ${migration} to verify rerunnability"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --file "$migration"
done

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --file .github/scripts/test_chat_migration.sql

# Exercise the production upgrade path separately: attest 0001-0019, then run
# the exact additive bootstrap bundle from 0020 through the current manifest.
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT USAGE ON SCHEMA public TO PUBLIC;
SQL
for migration in migrations/*.sql; do
  filename="${migration##*/}"
  version="${filename%%_*}"
  if (( 10#$version <= 19 )); then
    echo "Applying production baseline ${migration}"
    psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --file "$migration"
  fi
done
python3 .github/scripts/migration-ledger.py bootstrap \
  --baseline-through 0019 --through "$through" --output "$bundle"
echo "Applying generated production upgrade bundle from 0019 through ${through}"
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --file "$bundle"
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --file .github/scripts/test_chat_migration.sql

rls_count="$(psql "$DATABASE_URL" -Atc "select count(*) from pg_class c join pg_namespace n on n.oid=c.relnamespace where n.nspname='public' and c.relkind='r' and c.relrowsecurity")"
table_count="$(psql "$DATABASE_URL" -Atc "select count(*) from pg_class c join pg_namespace n on n.oid=c.relnamespace where n.nspname='public' and c.relkind='r'")"
if [ "$rls_count" -ne "$table_count" ]; then
  echo "RLS enabled on ${rls_count}/${table_count} public application tables" >&2
  exit 1
fi

client_grants="$(psql "$DATABASE_URL" -Atc "select count(*) from information_schema.table_privileges where table_schema='public' and grantee in ('PUBLIC','anon','authenticated')")"
if [ "$client_grants" -ne 0 ]; then
  echo "Found ${client_grants} unexpected direct-client table grants" >&2
  exit 1
fi

service_grants="$(psql "$DATABASE_URL" -Atc "select count(distinct table_name) from information_schema.table_privileges where table_schema='public' and grantee='service_role'")"
if [ "$service_grants" -ne "$table_count" ]; then
  echo "service_role has grants on ${service_grants}/${table_count} application tables" >&2
  exit 1
fi

printf 'Applied all migrations; verified RLS and backend-only grants on %s tables\n' "$table_count"
