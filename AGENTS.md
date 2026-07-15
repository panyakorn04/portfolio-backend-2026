## Stack notes

- Go 1.23 with go-zero `rest` framework (no zRPC)
- All persistence goes through Supabase REST/PostgREST — never direct SQL
- Admin auth uses bcrypt passwords, SHA-256 session tokens, and optional bearer tokens
- Rate limiting uses Redis when available, falls back to in-memory counters
- The config file (`etc/portfolio-api.yaml`) resolves `${ENV}` placeholders at startup
- Migrations are additive SQL files in `migrations/` — apply in numeric order
