#!/usr/bin/env bash
set -euo pipefail

: "${STAGED_OBSERVABILITY_DIR:?STAGED_OBSERVABILITY_DIR is required}"
: "${GRAFANA_LOKI_URL:?GRAFANA_LOKI_URL is required}"
: "${GRAFANA_LOKI_USERNAME:?GRAFANA_LOKI_USERNAME is required}"
: "${GRAFANA_LOKI_TOKEN:?GRAFANA_LOKI_TOKEN is required}"

case "$GRAFANA_LOKI_URL" in
  https://*/loki/api/v1/push) ;;
  *) echo "GRAFANA_LOKI_URL must be an HTTPS Loki push endpoint" >&2; exit 1 ;;
esac
[ -n "$GRAFANA_LOKI_USERNAME" ] || { echo "GRAFANA_LOKI_USERNAME is required" >&2; exit 1; }
[ -n "$GRAFANA_LOKI_TOKEN" ] || { echo "GRAFANA_LOKI_TOKEN is required" >&2; exit 1; }

OBSERVABILITY_DIR="${OBSERVABILITY_DIR:-/opt/apps/observability/alloy}"
COMPOSE_FILE="$OBSERVABILITY_DIR/docker-compose.yml"
ENV_FILE="$OBSERVABILITY_DIR/.env"

install -d -m 750 "$OBSERVABILITY_DIR"
# Docker may remap container UIDs. Keep the collector state directory writable
# across rootful and rootless/userns-remapped daemons. The parent remains 0750,
# and the sticky bit prevents one mapped UID from deleting another's files.
# This directory stores Alloy runtime state only; credentials stay in .env.
install -d -m 1777 "$OBSERVABILITY_DIR/data"
chmod 1777 "$OBSERVABILITY_DIR/data"
install -m 644 "$STAGED_OBSERVABILITY_DIR/config.alloy" "$OBSERVABILITY_DIR/config.alloy"
install -m 644 "$STAGED_OBSERVABILITY_DIR/docker-compose.yml" "$COMPOSE_FILE"

env_tmp=$(mktemp "$OBSERVABILITY_DIR/.env.XXXXXX")
# shellcheck disable=SC2329 # invoked by the EXIT trap
cleanup() {
  rm -f "$env_tmp"
  rm -rf "$STAGED_OBSERVABILITY_DIR"
}
trap cleanup EXIT
chmod 600 "$env_tmp"
printf 'GRAFANA_LOKI_URL=%s\nGRAFANA_LOKI_USERNAME=%s\nGRAFANA_LOKI_TOKEN=%s\n' \
  "$GRAFANA_LOKI_URL" "$GRAFANA_LOKI_USERNAME" "$GRAFANA_LOKI_TOKEN" > "$env_tmp"
mv "$env_tmp" "$ENV_FILE"

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config --quiet
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" pull --quiet
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --remove-orphans

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:12345/-/ready >/dev/null; then
    docker inspect --format '{{.State.Status}}' grafana-alloy
    exit 0
  fi
  sleep 2
done

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" logs --tail=100 grafana-alloy >&2
exit 1
