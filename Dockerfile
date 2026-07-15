# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.23-alpine@sha256:383395b794dffa5b53012a212365d40c8e37109a626ca30d6151c8348d380b5f AS builder

WORKDIR /app

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build the API binary (static, no CGO).
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/portfolio-api .

# ---- Runtime stage ----
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc

# CA certs for outbound HTTPS (webhooks, TLS Postgres), tzdata for timestamps.
# Alpine repositories replace superseded revisions, so pin the base digest and
# intentionally allow security package revisions to advance on clean rebuilds.
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /app/portfolio-api /app/portfolio-api
COPY --from=builder /app/swagger.json /app/swagger.json
COPY --from=builder /app/swagger.html /app/swagger.html
COPY etc /app/etc

USER app

EXPOSE 8888

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["wget", "--quiet", "--spider", "http://127.0.0.1:8888/api/health"]

STOPSIGNAL SIGTERM

# Config reads ${ENV} placeholders, so pass Supabase/env vars at runtime.
# See etc/portfolio-api.yaml.
ENTRYPOINT ["/app/portfolio-api"]
CMD ["-f", "etc/portfolio-api.yaml"]
