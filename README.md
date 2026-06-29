# Portfolio Backend (go-zero)

REST API for the portfolio site, ported from the Next.js server layer to
[go-zero](https://go-zero.dev). The frontend (Next.js) lives in a separate repo
and calls this service over HTTP.

## Tech stack

- Go 1.23
- go-zero `rest` (HTTP API, no zRPC)
- PostgreSQL via `database/sql` + pgx stdlib driver
- bcrypt password hashing, SHA-256 session-token hashing

## Layout

```text
.
├── etc/portfolio-api.yaml     # config (reads ${ENV} placeholders)
├── portfolio.api              # go-zero API spec (reference for goctl)
├── migrations/0001_init.sql   # Postgres schema (UUID PKs)
├── cmd/createuser/            # CLI to create/update a staff user
├── internal/
│   ├── config/                # config struct
│   ├── svc/                   # service context (DB pool)
│   ├── model/                 # SQL data access (6 tables)
│   ├── auth/                  # bcrypt, sessions, bearer, RBAC
│   ├── logic/                 # business logic (validation, webhook, ai, jobs)
│   ├── response/              # { ok, data } / { ok, error } envelopes
│   └── handler/               # HTTP handlers + routes
└── main.go
```

## Prerequisites

```bash
# Go toolchain
brew install go            # or download from https://go.dev/dl

# Optional: goctl, only needed to regenerate from portfolio.api
go install github.com/zeromicro/go-zero/tools/goctl@latest
export PATH=$PATH:$(go env GOPATH)/bin
```

## Setup

```bash
cp .env.example .env
# edit .env with your DATABASE_URL etc.

go mod tidy

# Apply the schema
psql "$DATABASE_URL" -f migrations/0001_init.sql

# Create an admin user
DATABASE_URL="$DATABASE_URL" go run ./cmd/createuser \
  -email you@example.com -password 'change-me' -role admin -name "You"
```

## Run

The config file reads `${ENV}` placeholders, so export the env first (e.g. via
`set -a; source .env; set +a`) then:

```bash
go run . -f etc/portfolio-api.yaml
# Starting portfolio-api at 0.0.0.0:8888...
```

## Docker

`docker compose` runs the API against the database configured in `.env`
(the existing Supabase instance) — it does not start a local Postgres:

```bash
cp .env.example .env   # set DATABASE_URL to your Supabase connection string
docker compose up --build
# API on http://localhost:8888
```

Build just the API image:

```bash
docker build -t portfolio-api .
docker run --rm -p 8888:8888 \
  -e DATABASE_URL="postgresql://user:pass@host:5432/portfolio?sslmode=disable" \
  -e NEXT_PUBLIC_SITE_URL="https://your-domain.com" \
  portfolio-api
```

The image is a multi-stage build (static `CGO_ENABLED=0` binary on Alpine,
runs as a non-root user). Pass all config via environment variables — the
config file resolves `${ENV}` placeholders at startup.

## Continuous integration and deployment

`.github/workflows/ci.yml` runs on pushes and PRs to `main`:

- **build-and-test** — `gofmt` check, `go vet`, `go build`, `go test -race`,
  then applies the migration against a Postgres service and smoke-tests
  `cmd/createuser`.
- **golangci-lint** — static analysis (config in `.golangci.yml`).
- **docker** — builds the Docker image (with GHA layer caching). On pushes to
  `main` and manual dispatches, it also publishes the image to GHCR as:
  - `ghcr.io/panyakorn04/portfolio-backend-2026:latest`
  - `ghcr.io/panyakorn04/portfolio-backend-2026:<commit-sha>`
- **deploy** — after the image is published on `main`, or when manually
  dispatched from `main`, SSHes into the VPS and restarts the `backend` Docker
  Compose service from `/opt/apps`.

Production deploy target:

- VPS: `76.13.185.117`
- App path: `/opt/apps`
- Backend Compose service: `backend`
- Caddy route: `http://76.13.185.117/api/`
- Postgres host from the app network: `postgres:5432`
- Redis host from the app network: `redis:6379`

Required GitHub Actions secrets:

- `VPS_HOST` — `76.13.185.117`.
- `VPS_USER` — SSH user on the VPS, for example `deploy`.
- `VPS_SSH_KEY` — private key allowed to SSH into the VPS.
- `BACKEND_IMAGE` — `ghcr.io/panyakorn04/portfolio-backend-2026:latest`.
- `GHCR_USERNAME` — optional for public GHCR packages, required if the VPS must
  authenticate to pull the private image.
- `GHCR_TOKEN` — optional for public GHCR packages, required if the VPS must
  authenticate to pull the private image. Use a GitHub PAT with `read:packages`.

The VPS Compose file should use the GHCR image for the backend service. This API
listens on port `8888`, so expose `8888` to the Compose network and point Caddy
at `backend:8888`:

```yaml
backend:
  image: ghcr.io/panyakorn04/portfolio-backend-2026:latest
  container_name: backend
  restart: unless-stopped
  environment:
    DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable
    REDIS_URL: redis://redis:6379
    NEXT_PUBLIC_SITE_URL: ${NEXT_PUBLIC_SITE_URL}
    CONTACT_WEBHOOK_URL: ${CONTACT_WEBHOOK_URL}
    CONTACT_WEBHOOK_SECRET: ${CONTACT_WEBHOOK_SECRET}
    ADMIN_API_TOKEN: ${ADMIN_API_TOKEN}
    INTERNAL_API_TOKEN: ${INTERNAL_API_TOKEN}
    AI_PROVIDER: ${AI_PROVIDER:-stub}
    AI_API_KEY: ${AI_API_KEY}
  expose:
    - "8888"
  depends_on:
    - postgres
    - redis
```

```caddy
:80 {
  handle_path /api/* {
    reverse_proxy backend:8888
  }

  handle {
    reverse_proxy frontend:80
  }
}
```

The deploy job runs:

```bash
cd /opt/apps
BACKEND_IMAGE="$BACKEND_IMAGE" docker compose pull backend
BACKEND_IMAGE="$BACKEND_IMAGE" docker compose up -d backend
docker image prune -f
```

## Endpoints

All responses use the shared envelope:

- success: `{ "ok": true, "data": ... }`
- error: `{ "ok": false, "error": { "message": ..., "details": [...] } }`

| Method | Path | Auth |
|--------|------|------|
| GET | `/api/health` | public |
| POST | `/api/contact` | public |
| GET | `/api/articles` | public |
| GET | `/api/articles/:slug` | public |
| GET/POST/DELETE | `/api/admin/session` | session/bearer |
| GET | `/api/admin/contact-inquiries` | admin |
| GET/PATCH | `/api/admin/contact-inquiries/:id` | admin (PATCH: admin/editor) |
| GET/DELETE | `/api/admin/sessions` | staff |
| DELETE | `/api/admin/sessions/:id` | staff |
| GET | `/api/admin/users` | admin |
| PATCH | `/api/admin/users/:id` | admin |
| GET/POST | `/api/admin/articles` | admin (POST: admin/editor) |
| GET/PATCH/DELETE | `/api/admin/articles/:id` | admin (write: admin/editor) |
| POST | `/api/ai/contact-summary` | admin |
| POST | `/api/jobs/contact-follow-up` | internal bearer |

## Database schema

The schema **matches the existing Prisma-generated database** (e.g. on Supabase)
so this service runs against the live data with no migration:

- Table names are PascalCase and quoted (`"User"`, `"Article"`, ...).
- Column names are camelCase and quoted (`"passwordHash"`, `"createdAt"`, ...).
- IDs are `text` (cuid) with **no DB default** — the app supplies a
  cuid-compatible ID on insert (see `internal/model/id.go`).
- Timestamps are `timestamp` (without time zone), as Prisma created them.

`migrations/0001_init.sql` reproduces this schema for a brand-new database. The
production database already has these tables (created by Prisma).

## Auth notes

- **Sessions**: login sets an httpOnly cookie `portfolio_admin_session` holding a
  random token; the SHA-256 hash is stored in `"AuthSession"`. 7-day expiry.
- **Bearer**: `ADMIN_API_TOKEN` / `INTERNAL_API_TOKEN` bypass role checks.
- **Passwords**: new users use bcrypt. Existing users created by the original
  Next.js backend used scrypt (`salt:key` hex) and **keep working** —
  `VerifyPassword` accepts both formats, so no re-hash is required.
- **CORS**: set `CorsOrigins` in the config to the frontend origin so the cookie
  flow works cross-origin.

## Regenerating from the spec (optional)

`portfolio.api` documents the routes. The handwritten handlers are the source of
truth; use goctl only if you want to scaffold fresh boilerplate:

```bash
goctl api go -api portfolio.api -dir ./_generated
```
