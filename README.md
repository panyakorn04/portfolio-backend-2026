# Portfolio Backend (go-zero)

REST API for the portfolio site, ported from the Next.js server layer to
[go-zero](https://go-zero.dev). The frontend (Next.js) lives in a separate repo
and calls this service over HTTP.

## Tech stack

- Go 1.23
- go-zero `rest` (HTTP API, no zRPC)
- Supabase REST/PostgREST for all persistence
- Optional Redis cache for public article GET responses
- bcrypt password hashing, SHA-256 session-token hashing

## Layout

```text
.
├── etc/portfolio-api.yaml     # config (reads ${ENV} placeholders)
├── portfolio.api              # go-zero API spec (reference for goctl)
├── migrations/0001_init.sql   # optional bootstrap schema for a brand-new Supabase project
├── cmd/createuser/            # CLI to create/update a staff user via Supabase REST
├── internal/
│   ├── config/                # config struct
│   ├── svc/                   # service context (Supabase REST clients)
│   ├── model/                 # Supabase REST data access
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
# edit .env with your Supabase URL and keys

go mod tidy

# Optional only for a brand-new Supabase database:
# apply migrations/0001_init.sql in the Supabase SQL editor.

# Create/update an admin user through Supabase REST
set -a; source .env; set +a
go run ./cmd/createuser \
  -email you@example.com -password 'change-me' -role admin -name "You"
```

Required Supabase env:

- `NEXT_PUBLIC_SUPABASE_URL`
- `NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY`
- `SUPABASE_SERVICE_ROLE_KEY` for backend/admin writes. The backend falls back to
  the publishable key when the service role key is empty, but production should
  provide the service role key from server-side secrets only.

Optional Redis article cache env:

- `REDIS_URL`, for example `redis://localhost:6379/0`. Leave empty to disable.
- `ARTICLE_CACHE_TTL_SECONDS`, defaults to 300 seconds when unset or invalid.

## Run

The config file reads `${ENV}` placeholders, so export the env first (e.g. via
`set -a; source .env; set +a`) then:

```bash
go run . -f etc/portfolio-api.yaml
# Starting portfolio-api at 0.0.0.0:8888...
```

## Docker

`docker compose` runs the API against Supabase REST configured in `.env`. It does
not start a local Postgres service:

```bash
cp .env.example .env   # set Supabase URL/keys
docker compose up --build
# API on http://localhost:8888
```

Build just the API image:

```bash
docker build -t portfolio-api .
docker run --rm -p 8888:8888 \
  -e NEXT_PUBLIC_SUPABASE_URL="https://your-project.supabase.co" \
  -e NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY="..." \
  -e SUPABASE_SERVICE_ROLE_KEY="..." \
  -e REDIS_URL="redis://host.docker.internal:6379/0" \
  -e ARTICLE_CACHE_TTL_SECONDS="300" \
  -e NEXT_PUBLIC_SITE_URL="https://your-domain.com" \
  portfolio-api
```

The image is a multi-stage build (static `CGO_ENABLED=0` binary on Alpine,
runs as a non-root user). Pass all config via environment variables — the
config file resolves `${ENV}` placeholders at startup.

## Continuous integration and deployment

`.github/workflows/ci.yml` runs on pushes and PRs to `main`:

- **build-and-test** — `gofmt` check, `go vet`, `go build`, `go test -race`.
- **golangci-lint** — static analysis (config in `.golangci.yml`).
- **docker** — builds the Docker image (with GHA layer caching). On pushes to
  `main` and manual dispatches, it also publishes the image to GHCR as:
  - `ghcr.io/panyakorn04/portfolio-backend-2026:latest`
  - `ghcr.io/panyakorn04/portfolio-backend-2026:<commit-sha>`
- **deploy** — after the image is published on `main`, or when manually
  dispatched from `main`, SSHes into the VPS, syncs app env from GitHub
  repository variables/secrets into `/opt/apps/.env`, then restarts the
  `backend` Docker Compose service from `/opt/apps`.

### GitHub-managed app env

Runtime app env is managed from GitHub repository settings:

- Variables: `Settings > Secrets and variables > Actions > Variables`
- Secrets: `Settings > Secrets and variables > Actions > Secrets`

Repository variables:

- `NEXT_PUBLIC_SUPABASE_URL`
- `NEXT_PUBLIC_SITE_URL`
- `REDIS_URL`
- `ARTICLE_CACHE_TTL_SECONDS`
- `CONTACT_WEBHOOK_URL`
- `AI_PROVIDER`
- `OLLAMA_BASE_URL`
- `OLLAMA_MODEL`

Repository secrets:

- `NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY`
- `SUPABASE_SERVICE_ROLE_KEY`
- `CONTACT_WEBHOOK_SECRET`
- `ADMIN_API_TOKEN`
- `INTERNAL_API_TOKEN`
- `AI_API_KEY`

During deploy, GitHub values are primary. If a value is empty in GitHub, the
workflow preserves the existing value already present in `/opt/apps/.env` so a
partial GitHub setup does not wipe production secrets.

Production deploy target:

- VPS: `76.13.185.117`
- App path: `/opt/apps`
- Backend Compose service: `backend`
- Caddy route: `http://76.13.185.117/api/`

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
at `backend:8888`. To let the deploy workflow override the image, use
`${BACKEND_IMAGE}` with the GHCR image as the default:

```yaml
backend:
  image: ${BACKEND_IMAGE:-ghcr.io/panyakorn04/portfolio-backend-2026:latest}
  container_name: backend
  restart: unless-stopped
  environment:
    NEXT_PUBLIC_SUPABASE_URL: ${NEXT_PUBLIC_SUPABASE_URL}
    NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY: ${NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY}
    SUPABASE_SERVICE_ROLE_KEY: ${SUPABASE_SERVICE_ROLE_KEY}
    REDIS_URL: ${REDIS_URL}
    ARTICLE_CACHE_TTL_SECONDS: ${ARTICLE_CACHE_TTL_SECONDS:-300}
    NEXT_PUBLIC_SITE_URL: ${NEXT_PUBLIC_SITE_URL}
    CONTACT_WEBHOOK_URL: ${CONTACT_WEBHOOK_URL}
    CONTACT_WEBHOOK_SECRET: ${CONTACT_WEBHOOK_SECRET}
    ADMIN_API_TOKEN: ${ADMIN_API_TOKEN}
    INTERNAL_API_TOKEN: ${INTERNAL_API_TOKEN}
    AI_PROVIDER: ${AI_PROVIDER:-stub}
    AI_API_KEY: ${AI_API_KEY}
    OLLAMA_BASE_URL: ${OLLAMA_BASE_URL:-http://ollama:11434}
    OLLAMA_MODEL: ${OLLAMA_MODEL:-panyakorn-local:latest}
  expose:
    - "8888"
```

AI chat endpoint:

```bash
curl -sS https://api.panyakorn.com/api/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"ตอบเป็นภาษาไทยสั้น ๆ ว่าพร้อมใช้งานไหม"}]}'
```

The endpoint forwards to the internal Ollama service configured by
`OLLAMA_BASE_URL`/`OLLAMA_MODEL`, caps request bodies/messages, and applies a
small in-memory per-client rate limit so the local VPS model cannot be hammered
unboundedly. Keep Ollama internal-only; do not publish port `11434`.

```caddy
:80 {
  handle /api/* {
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

## Supabase schema

The schema matches the existing Prisma-generated database on Supabase:

- Table names are PascalCase (`User`, `Article`, ...).
- Column names are camelCase (`passwordHash`, `createdAt`, ...).
- IDs are `text` (cuid) with no DB default — the app supplies a cuid-compatible
  ID on insert (see `internal/model/id.go`).
- Timestamps are `timestamp`/Supabase-compatible values.

`migrations/0001_init.sql` remains as an optional bootstrap script for a brand-new
Supabase project. Runtime access does not use direct SQL connections.

## Redis article cache

When `REDIS_URL` is set, the public article endpoints cache their full JSON
success response:

- `GET /api/articles?lang=...&limit=...`
- `GET /api/articles/:slug?lang=...`

The default TTL is 5 minutes (`ARTICLE_CACHE_TTL_SECONDS=300`). Cache failures are
non-fatal; the API falls back to Supabase REST. Admin article create/update/delete
clears `portfolio:articles:*` keys so public pages refresh after edits.

## Auth notes

- **Sessions**: login sets an httpOnly cookie `portfolio_admin_session` holding a
  random token; the SHA-256 hash is stored in `AuthSession`. 7-day expiry.
- **Bearer**: `ADMIN_API_TOKEN` / `INTERNAL_API_TOKEN` bypass role checks.
- **Passwords**: new users use bcrypt. Existing users created by the original
  Next.js backend used scrypt (`salt:key` hex) and keep working —
  `VerifyPassword` accepts both formats, so no re-hash is required.
- **CORS**: set `CorsOrigins` in the config to the frontend origin so the cookie
  flow works cross-origin.

## Regenerating from the spec (optional)

`portfolio.api` documents the routes. The handwritten handlers are the source of
truth; use goctl only if you want to scaffold fresh boilerplate:

```bash
goctl api go -api portfolio.api -dir ./_generated
```
