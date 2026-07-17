# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder

WORKDIR /app

# Keep downloaded modules in this stable builder layer. Unlike a cache mount,
# the module data is preserved by remote BuildKit layer caches used in CI.
COPY --link go.mod go.sum ./
RUN go mod download && go mod verify

# Copy only production Go sources so docs, tests, migrations, and local tooling
# do not invalidate the application build layer.
COPY --link main.go ./
COPY --link internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -buildvcs=false \
      -ldflags="-s -w -buildid=" \
      -o /out/portfolio-api .

# ---- Runtime stage ----
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# CA certs for outbound HTTPS (webhooks, TLS Postgres), tzdata for timestamps.
# Alpine repositories replace superseded revisions, so pin the base digest and
# intentionally allow security package revisions to advance on clean rebuilds.
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 app && \
    adduser -S -D -H -u 10001 -G app app

WORKDIR /app

COPY --link --from=builder /out/portfolio-api ./portfolio-api
COPY --link swagger.json swagger.html ./
COPY --link etc/ ./etc/

USER app

EXPOSE 8888

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["wget", "--quiet", "--spider", "http://127.0.0.1:8888/api/health"]

STOPSIGNAL SIGTERM

# Config reads ${ENV} placeholders, so pass Supabase/env vars at runtime.
# See etc/portfolio-api.yaml.
ENTRYPOINT ["/app/portfolio-api"]
CMD ["-f", "etc/portfolio-api.yaml"]
