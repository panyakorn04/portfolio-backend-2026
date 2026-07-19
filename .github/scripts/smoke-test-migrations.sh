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

for migration in migrations/*.sql; do
  echo "Applying ${migration}"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --file "$migration"
done

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
