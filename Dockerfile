# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.23-alpine AS builder

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
FROM alpine:3.20

# CA certs for outbound HTTPS (webhooks, TLS Postgres), tzdata for timestamps.
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /app/portfolio-api /app/portfolio-api
COPY etc /app/etc

USER app

EXPOSE 8888

# Config reads ${ENV} placeholders, so pass env vars at runtime
# (e.g. DATABASE_URL). See etc/portfolio-api.yaml.
ENTRYPOINT ["/app/portfolio-api"]
CMD ["-f", "etc/portfolio-api.yaml"]
