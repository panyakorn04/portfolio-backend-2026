#!/usr/bin/env bash
set -Eeuo pipefail

image="${1:?Usage: smoke-test-image.sh <image>}"
port="${SMOKE_PORT:-8888}"
platform="${SMOKE_PLATFORM:-linux/amd64}"
container_name="portfolio-backend-smoke-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}-$$"
container=""

# shellcheck disable=SC2329  # Invoked indirectly by the EXIT trap.
cleanup() {
  local exit_code=$?
  trap - EXIT
  if [ -n "$container" ]; then
    if [ "$exit_code" -ne 0 ]; then
      echo "Container logs after smoke-test failure:"
      docker logs "$container" --tail=100 2>&1 || true
      docker inspect "$container" --format='state={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' || true
    fi
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
  exit "$exit_code"
}
trap cleanup EXIT

container="$(docker run -d \
  --name "$container_name" \
  --platform "$platform" \
  --env ARTICLE_CACHE_TTL_SECONDS=300 \
  --env TRUST_PROXY=false \
  --publish "127.0.0.1:${port}:8888" \
  "$image")"

for attempt in {1..20}; do
  state="$(docker inspect "$container" --format='{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}')"
  read -r runtime_status health_status <<< "$state"

  case "$runtime_status:$health_status" in
    running:healthy)
      curl -fsS -o /dev/null --max-time 10 "http://127.0.0.1:${port}/api/health"
      echo "Backend image health check and HTTP probe passed: ${image}"
      exit 0
      ;;
    running:starting)
      echo "Waiting for backend image health check... attempt ${attempt}/20"
      sleep 3
      ;;
    running:missing)
      echo "Backend image does not define a Docker health check" >&2
      exit 1
      ;;
    *)
      echo "Backend image entered an unexpected state: ${state}" >&2
      exit 1
      ;;
  esac
done

echo "Backend image health check did not become ready: ${image}" >&2
exit 1
