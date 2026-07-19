# Portfolio Backend 2026

[![CI/CD](https://github.com/panyakorn04/portfolio-backend-2026/actions/workflows/ci.yml/badge.svg)](https://github.com/panyakorn04/portfolio-backend-2026/actions/workflows/ci.yml)

Production Go API for Panyakorn Boonyong's portfolio ecosystem. It serves the public portfolio, administration workspace, AI assistant, anonymous chat persistence, and AI Workflow Studio from one go-zero REST service.

Backend API สำหรับระบบ Portfolio ของ Panyakorn Boonyong ครอบคลุมบทความและแบบฟอร์มติดต่อ ระบบหลังบ้าน AI assistant การเก็บประวัติแชตแบบ anonymous และ workflow runtime ของ AI Workflow Studio โดยใช้ Supabase REST/PostgREST เป็น persistence layer หลัก

## Live services

| Service | URL |
| --- | --- |
| API | [api.panyakorn.com](https://api.panyakorn.com/api/health) |
| Swagger UI | [api.panyakorn.com/swagger](https://api.panyakorn.com/swagger) |
| Swagger 2.0 document | [api.panyakorn.com/swagger/doc.json](https://api.panyakorn.com/swagger/doc.json) |
| Portfolio frontend | [panyakorn.com](https://panyakorn.com) |

## What is included

### Portfolio platform

- Public health, contact, and localized article endpoints
- Contact inquiry persistence with an optional signed outbound webhook
- Admin session login/logout, session revocation, users, articles, and inquiries
- Anonymous portfolio chat sessions backed by Supabase
- Visitor-owned session lookup, session deletion, latest-session recovery, and human-handoff requests
- Persisted admin chat inbox with replies and status transitions
- Redis-backed public article caching with mutation invalidation

### AI adapter

- JSON and SSE chat endpoints backed by internal Ollama
- Public portfolio-assistant profile loaded from `AI_SKILLS_DIR`
- Pinned AI-console guardrail profile
- Server-side allowlist for selectable public chat models
- Admin-only Ollama model, runtime, version, model-detail, and embedding endpoints
- Public chat request limits and bounded payload/history handling
- Ollama remains internal; port `11434` must not be published directly

### AI Workflow Studio runtime

- Public workflow/execution overview and execution-stage projections
- Authenticated workflow CRUD with versioned graph definitions
- Manual, Schedule, and signed Webhook triggers
- Durable Supabase-backed execution queue with worker leases
- Persisted stage input/output, retry, cancellation, and audit logs
- SSE execution snapshots with polling, heartbeat, and bounded lifetime
- HTTP Request action nodes with SSRF/resource-limit hardening
- Secret-safe cURL import
- AES-256-GCM encrypted Studio credentials with scope-bound authenticated data
- Individually signed webhook capabilities
- Redis-backed mutation/login rate limiting with in-memory fallback

## Architecture

```text
portfolio-2026              ai-workflow-studio             open-webui-theme
     │                               │                            │
     └────────────── HTTPS /api/* ───┴────────────────────────────┘
                                     │
                                     ▼
                         portfolio-backend-2026
                         Go 1.26.5 + go-zero REST
                          │         │          │
                          │         │          └── Ollama
                          │         │              local models
                          │         │
                          │         └── Redis
                          │             cache + rate limits
                          │
                          └── Supabase REST/PostgREST
                              data + RPC-backed workflow queue
```

The service does not open a direct PostgreSQL connection. All runtime persistence goes through Supabase REST/PostgREST and database RPCs defined by the ordered SQL migrations.

## Technology

| Layer | Technology |
| --- | --- |
| Language | Go 1.26.5 |
| HTTP framework | go-zero `rest` |
| Persistence | Supabase REST/PostgREST |
| Cache and distributed limits | Redis 7 via `go-redis/v9` |
| AI runtime | Ollama HTTP API |
| Authentication | bcrypt, legacy scrypt compatibility, SHA-256 session-token hashes, bearer tokens |
| Credential encryption | AES-256-GCM with scope-bound AAD |
| API documentation | `portfolio.api`, committed partial `swagger.json`, and hosted Swagger UI |
| Container | Multi-stage Alpine image, static binary, non-root runtime user |
| Delivery | GitHub Actions, GHCR immutable SHA images, Docker Compose, Caddy |

## Repository structure

```text
.
├── main.go                         # service bootstrap and Studio worker
├── portfolio.api                   # go-zero API contract/reference
├── swagger.json                    # hosted API document
├── swagger.html                    # hosted Swagger UI shell
├── etc/portfolio-api.yaml          # go-zero runtime configuration
├── cmd/createuser/                 # staff-user create/update CLI
├── deploy/
│   ├── README.md                   # production deployment contract
│   └── deploy-compose-service.sh   # lock, deploy, health gate, rollback
├── internal/
│   ├── auth/                       # sessions, bearer auth, password verification, RBAC
│   ├── cache/                      # Redis article cache and counters
│   ├── config/                     # typed go-zero config
│   ├── handler/                    # routes, HTTP handlers, Studio runtime and SSE
│   ├── logic/                      # articles, contact, webhook, and job logic
│   ├── model/                      # Supabase REST models, Ollama, skill profiles
│   ├── response/                   # shared JSON response envelopes
│   ├── security/                   # Studio credential cipher
│   └── svc/                        # service context and dependencies
├── migrations/                    # additive Supabase SQL, apply in numeric order
├── .github/workflows/
│   ├── ci.yml                      # validate, lint, image, deploy, rollback
│   └── pull-ollama-model.yml       # manual model installation on the VPS
├── Dockerfile
├── docker-compose.yml              # local API + Redis development stack
└── Makefile
```

The repository also contains project-specific agent skills under `.agents/skills/`; they are development guidance and are not part of the runtime image contract.

## Prerequisites

- Go 1.26.5+
- A Supabase project
- Docker and Docker Compose for container-based development
- Redis is optional for direct local runs and included by the local Compose stack
- Ollama is optional unless exercising AI endpoints
- `goctl` is optional and only needed for scaffolding from `portfolio.api`

```bash
brew install go

# Optional
go install github.com/zeromicro/go-zero/tools/goctl@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Local setup

```bash
git clone https://github.com/panyakorn04/portfolio-backend-2026.git
cd portfolio-backend-2026
cp .env.example .env
```

Set your Supabase URL and keys in `.env`. For a clean Supabase project, apply every file in `migrations/` through the Supabase SQL Editor in numeric order before creating users or starting features that depend on those tables.

The CLI intentionally creates only `editor` or `viewer` accounts:

```bash
set -a
source .env
set +a

go run ./cmd/createuser \
  -email you@example.com \
  -password 'replace-this-password' \
  -role editor \
  -name 'Your Name'
```

For the first installation, promote that user to `admin` directly in the Supabase dashboard. After bootstrap, an existing admin can manage roles through the admin API. The CLI rejects `-role admin` by design.

Run the API:

```bash
make dev
# http://localhost:8888
# http://localhost:8888/swagger
```

Equivalent direct command:

```bash
set -a; source .env; set +a
go run . -f etc/portfolio-api.yaml
```

## Environment contract

Never commit `.env` or production credentials. `.env.example` contains names and safe development defaults only.

### Persistence and site

| Variable | Purpose |
| --- | --- |
| `NEXT_PUBLIC_SUPABASE_URL` | Supabase project URL |
| `NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY` | Publishable REST key; development/read fallback |
| `SUPABASE_SERVICE_ROLE_KEY` | Backend-only key for writes, sessions, admin, chat, and Studio operations |
| `NEXT_PUBLIC_SITE_URL` | Public portfolio origin and secure-cookie decision |
| `REDIS_URL` | Optional Redis connection; empty disables cache/distributed limits |
| `ARTICLE_CACHE_TTL_SECONDS` | Public article cache TTL; defaults to 300 seconds |
| `TRUST_PROXY` | Trust proxy-overwritten client-IP headers for rate limits; keep `false` for direct local exposure and use `true` behind production Caddy |

Production should always provide `SUPABASE_SERVICE_ROLE_KEY`. The service can initialize with the publishable key when the service-role key is empty, but privileged writes may fail under RLS and that fallback is not a production configuration.

### Authentication, Studio, and contact

| Variable | Purpose |
| --- | --- |
| `ADMIN_API_TOKEN` | Optional static admin bearer token; bypasses session RBAC |
| `INTERNAL_API_TOKEN` | Bearer token for internal job endpoints |
| `STUDIO_CREDENTIAL_ENCRYPTION_KEY` | Base64-encoded 32-byte key for Studio credential encryption |
| `STUDIO_WEBHOOK_SIGNING_KEY` | Independent HMAC key for Studio webhook capabilities |
| `CONTACT_WEBHOOK_URL` | Optional destination for contact-submission notifications |
| `CONTACT_WEBHOOK_SECRET` | Optional signing secret for the contact webhook |

Generate independent keys; do not reuse bearer tokens:

```bash
openssl rand -base64 32  # STUDIO_CREDENTIAL_ENCRYPTION_KEY
openssl rand -base64 48  # STUDIO_WEBHOOK_SIGNING_KEY or visitor secret
```

### AI and portfolio chat

| Variable | Purpose |
| --- | --- |
| `AI_PROVIDER` | Provider mode; defaults to `stub` in `.env.example` |
| `AI_API_KEY` | Reserved credential input for the contact-summary provider boundary; the current contact-summary implementation remains stub-only |
| `OLLAMA_BASE_URL` | Internal Ollama API URL |
| `OLLAMA_MODEL` | Default Ollama model |
| `OLLAMA_ALLOWED_MODELS` | Comma-separated public chat model allowlist; defaults to the pinned model when empty |
| `AI_SKILLS_DIR` | Root directory for mounted AI skill profiles |
| `PORTFOLIO_CHAT_VISITOR_SECRET` | HMAC key used before visitor identifiers reach the database |
| `PORTFOLIO_CHAT_SESSION_TTL_HOURS` | Intended anonymous-session lifetime; production default is 2160 hours |
| `PORTFOLIO_CHAT_MAX_STORED_MESSAGES` | Intended per-session history cap; production default is 100 |

The two portfolio-chat numeric defaults are currently set in `etc/portfolio-api.yaml`. If you change their environment values, keep the runtime config mapping synchronized.

## Docker development

The local Compose file starts the API and Redis. It does not start PostgreSQL; persistence still uses the configured Supabase project.

```bash
cp .env.example .env
docker compose up --build
```

Services:

- API: `http://localhost:8888`
- Redis: `localhost:6379`

Build only the production image:

```bash
docker build -t portfolio-backend-2026 .
docker run --rm --env-file .env -p 8888:8888 portfolio-backend-2026
```

The runtime image contains the static API binary, go-zero config, Swagger document, and Swagger UI. It runs as the non-root `app` user.

## Commands

| Command | Purpose |
| --- | --- |
| `make dev` | Load `.env` and run the API |
| `make build` | Build `./portfolio-backend` |
| `make clean` | Remove the local binary |
| `go test ./...` | Run all tests |
| `go test ./... -race -count=1` | Run the CI race-enabled test gate |
| `go vet ./...` | Run Go vet |
| `golangci-lint run --timeout=5m` | Run the configured lint gate |
| `gofmt -w <files>` | Format changed Go files |

`portfolio.api` is a contract/reference and the handwritten handlers in `internal/handler/routes.go` are the runtime route source of truth. The current Swagger 2.0 document does not yet represent every newer admin-chat and portfolio human-handoff route, so use the runtime route table below for those surfaces. Keep `portfolio.api`, `swagger.json`, and the handlers synchronized when endpoints change.

Optional go-zero scaffolding:

```bash
goctl api go -api portfolio.api -dir ./_generated
```

## API overview

All application JSON responses use a shared envelope:

```json
{ "ok": true, "data": {} }
```

```json
{ "ok": false, "error": { "message": "...", "details": [] } }
```

Use the hosted [Swagger UI](https://api.panyakorn.com/swagger) for schemas represented by `portfolio.api`. The table below is sourced from `internal/handler/routes.go` and summarizes the complete runtime route groups.

### Public portfolio and AI

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/health` | Service health |
| `GET` | `/api/ready` | Fail-closed deployment readiness check for live Studio persistence tables |
| `POST` | `/api/contact` | Validate and persist a contact inquiry |
| `GET` | `/api/articles` | List localized public articles |
| `GET` | `/api/articles/:slug` | Read a localized article |
| `POST` | `/api/ai/chat` | AI-console JSON chat with allowlisted model selection |
| `POST` | `/api/ai/chat/stream` | AI-console SSE chat |
| `POST` | `/api/portfolio/assistant/chat` | Public portfolio-assistant JSON chat |
| `POST` | `/api/portfolio/assistant/chat/stream` | Public portfolio-assistant SSE chat with optional persistence |
| `GET` | `/api/portfolio/assistant/sessions/current` | Current visitor-owned session |
| `GET` | `/api/portfolio/assistant/sessions/latest` | Latest visitor-owned session |
| `POST` | `/api/portfolio/assistant/sessions` | Create an anonymous session |
| `POST` | `/api/portfolio/assistant/sessions/:id/request-human` | Request human follow-up for an owned session |
| `DELETE` | `/api/portfolio/assistant/sessions/:id` | Delete an owned session |
| `POST` | `/api/ai/generate` | One-shot generation using the default model |

### Public Studio projections and triggers

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/studio/overview` | Public-safe workflow and execution overview |
| `GET` | `/api/studio/executions/:id/stages` | Public-safe stage projection |
| `GET` | `/api/studio/executions/:id/events` | SSE execution snapshots |
| `GET`, `POST` | `/api/studio/webhooks/:id/:nodeId` | Signed webhook trigger using `X-Studio-Webhook-Token` |

### Administration

Admin routes accept either the `portfolio_admin_session` cookie or `Authorization: Bearer <ADMIN_API_TOKEN>`. Session-backed calls enforce `admin`, `editor`, and `viewer` roles; static admin bearer access bypasses role checks.

| Group | Representative paths |
| --- | --- |
| Session | `/api/admin/session` |
| Inquiries | `/api/admin/contact-inquiries`, `/api/admin/contact-inquiries/:id` |
| Chat inbox | `/api/admin/chat/sessions`, `/:id`, `/:id/reply` |
| Session management | `/api/admin/sessions`, `/api/admin/sessions/:id` |
| Users | `/api/admin/users`, `/api/admin/users/:id` |
| Articles | `/api/admin/articles`, `/api/admin/articles/:id` |
| Studio workflows | `/api/admin/studio/workflows`, `/workflows/:id` |
| Studio execution | `/api/admin/studio/executions`, `/executions/:id`, `/retry`, `/cancel` |
| Studio node tools | `/nodes/:nodeId/execute`, `/execute-previous`, `/http-request` |
| Studio credentials | `/api/admin/studio/credentials`, `/:id`, `/:id/test` |
| Studio audit | `/api/admin/studio/audit-logs` |
| Ollama administration | `/api/ai/models`, `/running`, `/version`, `/model/show`, `/embed` |

`pause` and `approve` execution routes remain registered as explicit legacy command shapes but fail closed with `409`; they are not available runtime transitions.

The internal endpoint `POST /api/jobs/contact-follow-up` requires `INTERNAL_API_TOKEN` bearer authentication.

## Persistence and migrations

Supabase tables retain the original Prisma-compatible naming convention:

- PascalCase tables such as `User`, `Article`, and `AuthSession`
- camelCase columns such as `passwordHash` and `createdAt`
- application-generated text IDs

Apply SQL files from `migrations/` in numeric order. They are additive and cover:

1. Base portfolio/admin schema
2. Anonymous portfolio chat sessions and messages
3. Studio workflows, executions, stages, audit logs, and seed data
4. Versioned workflow definitions and durable graph execution RPCs
5. Encrypted Studio credentials
6. Schedule/webhook and execution-ownership security hardening
7. Admin chat access and status operations
8. Markdown article content as the authoritative body, with escaped legacy-section backfill
9. Backend-only table grants and row-level-security enforcement for all application data

GitHub Actions does not apply database migrations. Apply and verify each required migration in Supabase before deploying code that depends on it.

All runtime persistence is server-side through the Supabase `service_role`. Migration `0018_backend_table_access_security.sql` revokes direct `PUBLIC`, `anon`, and `authenticated` access to every application table, enables RLS as defense in depth, and establishes backend-only default table privileges for future migrations. Any future direct-client table must opt in explicitly with reviewed RLS policies.

## Authentication and security boundaries

- New passwords use bcrypt; legacy `salt:key` scrypt hashes remain verifiable for migration compatibility.
- Login creates a random 32-byte session token in an httpOnly `portfolio_admin_session` cookie; only its SHA-256 hash is stored.
- Admin sessions expire after seven days.
- Session-based authorization enforces role permissions; `ADMIN_API_TOKEN` is intentionally privileged and bypasses RBAC.
- Internal jobs use a separate `INTERNAL_API_TOKEN`.
- Anonymous portfolio visitors are represented by an HMAC-derived identifier, not the raw browser visitor ID.
- Studio credential ciphertext uses AES-256-GCM and scope-bound AAD; secret values never belong in public DTOs.
- Studio webhook capabilities use an independent signing key and a request header, never a query-string token.
- Public execution projections are sanitized separately from authenticated execution detail.
- Enable trusted proxy handling only behind a proxy that overwrites forwarded client-IP headers. Login, Studio, and public AI rate-limit keys ignore forwarded headers unless `TrustProxy` is enabled.
- CORS origins are configured in `etc/portfolio-api.yaml`.

## Article cache

When Redis is enabled, successful public article responses are cached for:

- `GET /api/articles?lang=...&limit=...`
- `GET /api/articles/:slug?lang=...`

The default TTL is five minutes. Redis failures are non-fatal for article reads; the API falls back to Supabase REST. Admin article mutations clear `portfolio:articles:*` keys.

## Application logging

The API emits structured JSON logs to stdout so Docker can collect them without a writable log volume. Runtime configuration is explicit in `etc/portfolio-api.yaml`: console mode, JSON encoding, `info` level, and an 8 KiB per-entry content limit.

A sanitized HTTP middleware emits one `http.request.completed` event per completed request with bounded metadata only:

```text
event, request_id, method, route, status, duration_ms
```

Valid incoming `X-Request-ID` values are propagated through request context, included in application error events, and returned in the response; missing or unsafe values are replaced. CORS preflights are logged too, and `X-Request-ID` is included in the allowed/exposed CORS headers. Request logs use registered route patterns rather than raw URL paths, so resource identifiers and article slugs are not emitted. The logger wraps CORS and the go-zero router, so native timeout and panic-recovery responses are recorded with their final status. The middleware never records query strings, request/response bodies, authorization headers, or cookies. The built-in go-zero request logger remains disabled because its 5xx path can dump request headers and bodies. Tracing, Prometheus, recovery, timeout, and the other native middleware remain enabled.

Application failures use context-aware `logx` events so go-zero trace/span correlation is retained. Event names are stable dotted identifiers such as `studio.execution.enqueue_failed`, `portfolio_chat.session.create_failed`, and `ollama.generate.failed`. Operational logs record only the Go `error_type`; arbitrary `err.Error()` text is never serialized because dependency response bodies may contain tokens, visitor identifiers, PII, or raw AI content.

Never log passwords, bearer/session tokens, cookies, Supabase keys, Studio credentials, webhook capabilities, visitor identifiers, raw AI messages, or arbitrary request bodies. `StudioAuditLog` remains the immutable business audit trail and is intentionally separate from operational stdout logs.

Local inspection:

```bash
docker compose logs -f api
```

Production ships the same stdout stream to Grafana Cloud Loki through Grafana Alloy. The deployment is defined in `observability/alloy/` and is installed with the manual `Deploy Grafana Alloy` workflow. Configure these repository secrets before dispatching it:

```text
GRAFANA_LOKI_URL       # HTTPS endpoint ending in /loki/api/v1/push
GRAFANA_LOKI_USERNAME  # Grafana Cloud hosted-logs instance ID
GRAFANA_LOKI_TOKEN     # access-policy token with logs:write and logs:read
```

The workflow transfers credentials over pinned SSH stdin, stores the production Alloy environment file with mode `0600`, starts a digest-pinned Alloy v1.17.1 container, probes the local Alloy readiness endpoint, and requires the exact structured request-ID event from the production backend. Alloy reads only the `api`/`backend` Docker Compose services and assigns bounded `application="portfolio-api"` and `environment="production"` labels; request IDs remain inside log content and never become high-cardinality labels.

Example Grafana Explore query:

```logql
{application="portfolio-api", environment="production"} | json
```

### AI-assisted production alerts

`.github/workflows/production-log-monitor.yml` queries the bounded production Loki stream every five minutes. Detection is deterministic: any HTTP status `>= 500`, error/fatal/panic level, or trusted stable `*.failed` event opens an incident. The workflow derives exact route and event allowlists from the authoritative `internal/handler/routes.go` registration table and trusted Go observability call sites, replaces every unknown value with `redacted`, and reduces arbitrary Go error types to the fixed value `present`. It sends only aggregate counts for statuses, allowlisted registered route patterns, allowlisted event names, and error-type presence to the configured portfolio Ollama endpoint for a short Thai summary; raw log lines, request IDs, bodies, headers, cookies, error strings/types, and user data are never sent to AI or Discord.

The first incident sends a Discord alert, a newly observed status/route/event/error-presence signature sends an immediate follow-up, an unchanged active incident is reminded at most every 30 minutes, and the first clean interval sends a recovery notification. The signature is calculated from every observed dimension before the top-ten presentation limit, so a low-frequency new route or event is not suppressed. Loki responses that reach the page limit are split into smaller bounded time ranges instead of being treated as complete; missing, malformed, unparseable, unsplittable, truncated, or unavailable query results fail closed and send a sanitized monitor-failure alert without changing incident state. State is carried between workflow runs through a seven-day GitHub Actions artifact. Configure these repository settings:

```text
Secret:   DISCORD_WEBHOOK_URL
Variable: AI_LOG_SUMMARY_URL=https://api.panyakorn.com/api/ai/generate
```

Use **Production Log Monitor → Run workflow → dry_run=true** to verify Loki access and the sanitized aggregate without sending Discord or updating incident state. To verify the complete AI-to-Discord path safely, set `dry_run=false` and `send_test_alert=true`; the message is explicitly labeled as synthetic and does not change incident state.

## CI/CD and production deployment

`.github/workflows/ci.yml` runs for pull requests, pushes to `main`, and manual dispatches.

### Validation

- `gofmt` cleanliness
- `go vet ./...`
- `go test ./... -race -count=1 -shuffle=on` (compiles every package, so CI no longer repeats a separate `go build`)
- `bash -n` for deployment and image-smoke scripts
- `golangci-lint` v2.12.2 with its native v2 configuration
- Docker Buildx `linux/amd64` image build with a reusable GitHub Actions cache
- Trivy fail-closed scanning for fixed high/critical OS and Go dependency vulnerabilities
- Exact-image Docker health check plus an external `/api/health` HTTP probe before deployment

Stale runs are cancelled per pull-request ref, while runs on the same `main` or manual-dispatch ref remain serialized. Third-party actions are pinned to immutable commit SHAs and use Node 24-compatible releases.

### Immutable release flow

For non-PR runs, the image is built and loaded locally, scanned, and smoke-tested before it is published only as:

```text
ghcr.io/panyakorn04/portfolio-backend-2026:<full-commit-sha>
```

There is no mutable `latest` deployment tag. Production deploys select the exact commit image, authenticate to GHCR with the short-lived repository `GITHUB_TOKEN`, and remove the temporary Docker credential directory afterward.

Before the protected `production` approval, rollout preflight verifies the immutable release image, pullable previous successful image, required repository assets, and migration state. Pushes that add migrations stop before approval; after applying and verifying those migrations manually, rerun with `migration_verified` enabled. Every production deploy and rollback then waits for a required reviewer to approve the protected `production` environment; there is no time-based deployment window or emergency time override.

The versioned deploy script:

1. Acquires `/opt/apps/.production-deploy.lock`.
2. Preserves the existing `/opt/apps/.env` and updates only `BACKEND_IMAGE`.
3. Validates the Compose stack.
4. Verifies that the previous release is still pullable before changing production.
5. Pulls and force-recreates only the `backend` service, then confirms that the running container uses the requested immutable image.
6. Gates success on immediate, short-term, and medium-term checks of `https://api.panyakorn.com/api/ready`, including a latency threshold. The endpoint returns an error unless the live Studio persistence tables can be queried.
7. Restores, force-recreates, and verifies the prior image automatically if deployment or sustained health verification fails.
8. Records durable rollout state on the VPS so an SSH disconnect is reconciled before CI decides whether a retry is safe; an ambiguous in-progress transaction is never replayed blindly.

Deployment uses these externally provisioned Compose files:

```text
/opt/apps/docker-compose.yml
/opt/apps/docker-compose.ai-skills.yml
/opt/apps/docker-compose.studio.yml
```

The repository does not contain the complete production Compose/Caddy stack or production application secrets.

### Required production GitHub secrets

```text
VPS_HOST
VPS_USER
VPS_SSH_KEY
VPS_KNOWN_HOSTS
```

`DISCORD_WEBHOOK_URL` is optional; when configured, the deployment sends start, success, failure, and rollback-result notifications. A GitHub job summary is always written even when no webhook is configured.

`VPS_KNOWN_HOSTS` must contain pinned OpenSSH `known_hosts` line(s), not only a fingerprint. The workflow does not sync application secrets from GitHub; production runtime variables remain in `/opt/apps/.env` and must retain least-privilege file permissions.

### Required GitHub Actions variables

```text
BACKEND_IMAGE_REPOSITORY
BACKEND_HEALTH_URL
BACKEND_COMPOSE_FILES
BACKEND_COMPOSE_SERVICE
BACKEND_IMAGE_VARIABLE
```

These operational values are configured in repository Settings rather than hardcoded into the workflow.

### Manual rollback

Run **CI/CD → Run workflow**, choose `rollback`, and provide a previously published full lowercase 40-character commit SHA. Image building is skipped and the same health-gated deploy script switches to that immutable release.

### Pulling an Ollama model

The manual **Pull Ollama Model** workflow installs a requested model in the running VPS Ollama container through the same pinned `VPS_KNOWN_HOSTS` trust boundary as the main deployment. It does not expose Ollama publicly.

## Related repositories

- [portfolio-2026](https://github.com/panyakorn04/portfolio-2026) — bilingual portfolio and admin frontend
- [ai-workflow-studio](https://github.com/panyakorn04/ai-workflow-studio) — workflow control-plane frontend
- [open-webui-theme](https://github.com/panyakorn04/open-webui-theme) — custom AI console frontend
- [custom-ai-skills](https://github.com/panyakorn04/custom-ai-skills) — mounted AI skill profiles

## License

No open-source license is currently included. Unless a license is added, the source remains all rights reserved by default.
