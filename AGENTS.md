<!-- BEGIN:backend-skills -->
# Backend skills — use them

This repo ships project skills in `.agents/skills/`. Activate them proactively (no need to ask first) when the work matches:

- **supabase** — Supabase client: auth, storage, realtime, edge functions, and migrations. Use when working with Supabase REST queries, auth flows, or schema changes.
- **supabase-postgres-best-practices** — Postgres patterns for Supabase: schema design, RLS, indexing, and query performance. Use when designing migrations or optimizing queries.

## Stack notes

- Go 1.23 with go-zero `rest` framework (no zRPC)
- All persistence goes through Supabase REST/PostgREST — never direct SQL
- Admin auth uses bcrypt passwords, SHA-256 session tokens, and optional bearer tokens
- Rate limiting uses Redis when available, falls back to in-memory counters
- The config file (`etc/portfolio-api.yaml`) resolves `${ENV}` placeholders at startup
- Migrations are additive SQL files in `migrations/` — apply in numeric order
<!-- END:backend-skills -->
