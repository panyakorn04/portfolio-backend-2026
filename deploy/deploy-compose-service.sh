#!/usr/bin/env bash
set -Eeuo pipefail

: "${DEPLOY_IMAGE:?DEPLOY_IMAGE is required}"
: "${IMAGE_VARIABLE:?IMAGE_VARIABLE is required}"
: "${SERVICE:?SERVICE is required}"
: "${HEALTH_URL:?HEALTH_URL is required}"
: "${GHCR_USERNAME:?GHCR_USERNAME is required}"
: "${GHCR_TOKEN:?GHCR_TOKEN is required}"

APPS_DIR="${APPS_DIR:-/opt/apps}"
ENV_FILE="${ENV_FILE:-$APPS_DIR/.env}"
COMPOSE_FILES="${COMPOSE_FILES:-docker-compose.yml}"
HEALTH_ATTEMPTS="${HEALTH_ATTEMPTS:-20}"
HEALTH_DELAY="${HEALTH_DELAY:-5}"

cd "$APPS_DIR"
[ -f "$ENV_FILE" ] || touch "$ENV_FILE"
# GitHub concurrency is repository-scoped; serialize all repositories on the VPS.
exec 9>"$APPS_DIR/.production-deploy.lock"
flock -w 600 9 || { echo "Timed out waiting for the production deployment lock" >&2; exit 1; }

compose_args=()
for file in $COMPOSE_FILES; do
  [ -f "$file" ] || { echo "Missing Compose file: $file" >&2; exit 1; }
  compose_args+=( -f "$file" )
done

registry="${DEPLOY_IMAGE%%/*}"
docker_config="$(mktemp -d)"
env_backup="$(mktemp)"
cp "$ENV_FILE" "$env_backup"
export DOCKER_CONFIG="$docker_config"
cleanup() { rm -rf "$docker_config" "$env_backup"; }
trap cleanup EXIT
printf '%s\n' "$GHCR_TOKEN" | docker login "$registry" --username "$GHCR_USERNAME" --password-stdin
unset GHCR_TOKEN

previous_image="$(awk -F= -v key="$IMAGE_VARIABLE" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$ENV_FILE")"
if [ -z "$previous_image" ]; then
  container_id="$(docker compose "${compose_args[@]}" ps -q "$SERVICE" 2>/dev/null || true)"
  if [ -n "$container_id" ]; then
    previous_image="$(docker inspect --format='{{.Config.Image}}' "$container_id" 2>/dev/null || true)"
  fi
fi

set_image() {
  local image="$1" tmp
  tmp="$(mktemp "${ENV_FILE}.XXXXXX")"
  awk -F= -v key="$IMAGE_VARIABLE" '$1 != key {print}' "$ENV_FILE" > "$tmp"
  printf '%s=%s\n' "$IMAGE_VARIABLE" "$image" >> "$tmp"
  chmod --reference="$ENV_FILE" "$tmp" 2>/dev/null || chmod 600 "$tmp"
  mv "$tmp" "$ENV_FILE"
}

healthy() {
  local attempt
  for ((attempt=1; attempt<=HEALTH_ATTEMPTS; attempt++)); do
    if curl --fail --silent --show-error --max-time 10 "$HEALTH_URL" >/dev/null; then
      return 0
    fi
    echo "Health check $attempt/$HEALTH_ATTEMPTS failed; retrying in ${HEALTH_DELAY}s"
    sleep "$HEALTH_DELAY"
  done
  return 1
}

rollback() {
  local status=$?
  trap - ERR
  echo "Deployment failed (status $status); restoring previous release" >&2
  if [ -n "$previous_image" ]; then
    set_image "$previous_image"
    docker compose "${compose_args[@]}" pull "$SERVICE"
    docker compose "${compose_args[@]}" up -d --no-deps "$SERVICE"
    if healthy; then
      echo "Automatic rollback succeeded: $previous_image" >&2
    else
      echo "Automatic rollback health check failed" >&2
    fi
  else
    cp "$env_backup" "$ENV_FILE"
    echo "No previous image was discoverable; restored .env only" >&2
  fi
  exit "$status"
}
trap rollback ERR

set_image "$DEPLOY_IMAGE"
docker compose "${compose_args[@]}" config --quiet
docker compose "${compose_args[@]}" pull "$SERVICE"
docker compose "${compose_args[@]}" up -d --no-deps "$SERVICE"
healthy
trap - ERR
printf '%s\n' "$DEPLOY_IMAGE" > "$APPS_DIR/.${SERVICE}-last-successful-image"
echo "Deployment healthy: $DEPLOY_IMAGE"
