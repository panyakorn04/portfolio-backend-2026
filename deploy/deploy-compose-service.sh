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
IMMEDIATE_ATTEMPTS="${IMMEDIATE_ATTEMPTS:-20}"
IMMEDIATE_DELAY="${IMMEDIATE_DELAY:-3}"
SHORT_TERM_ATTEMPTS="${SHORT_TERM_ATTEMPTS:-3}"
SHORT_TERM_DELAY="${SHORT_TERM_DELAY:-30}"
MEDIUM_TERM_ATTEMPTS="${MEDIUM_TERM_ATTEMPTS:-5}"
MEDIUM_TERM_DELAY="${MEDIUM_TERM_DELAY:-60}"
ROLLBACK_ATTEMPTS="${ROLLBACK_ATTEMPTS:-10}"
ROLLBACK_DELAY="${ROLLBACK_DELAY:-3}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-10}"
HEALTH_LATENCY_LIMIT="${HEALTH_LATENCY_LIMIT:-5}"

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

# Fail before touching production when the rollback artifact is unavailable.
if [ -n "$previous_image" ]; then
  echo "Verifying rollback image is pullable: $previous_image"
  docker pull "$previous_image" >/dev/null
  echo "ROLLBACK_WORTHINESS=verified"
else
  echo "No previous image was discoverable; this deployment has no image rollback path" >&2
  echo "ROLLBACK_WORTHINESS=unavailable"
fi

set_image() {
  local image="$1" tmp
  tmp="$(mktemp "${ENV_FILE}.XXXXXX")"
  awk -F= -v key="$IMAGE_VARIABLE" '$1 != key {print}' "$ENV_FILE" > "$tmp"
  printf '%s=%s\n' "$IMAGE_VARIABLE" "$image" >> "$tmp"
  chmod --reference="$ENV_FILE" "$tmp" 2>/dev/null || chmod 600 "$tmp"
  mv "$tmp" "$ENV_FILE"
}

probe() {
  local phase="$1" result latency
  if ! result="$(curl --fail --silent --show-error --output /dev/null \
    --max-time "$HEALTH_TIMEOUT" --write-out '%{http_code} %{time_total}' "$HEALTH_URL")"; then
    echo "${phase}: health request failed" >&2
    return 1
  fi
  latency="${result#* }"
  if ! awk -v actual="$latency" -v limit="$HEALTH_LATENCY_LIMIT" 'BEGIN { exit !(actual <= limit) }'; then
    echo "${phase}: latency ${latency}s exceeded ${HEALTH_LATENCY_LIMIT}s" >&2
    return 1
  fi
  echo "${phase}: HTTP ${result%% *}, latency ${latency}s"
}

verify_running_image() {
  local expected="$1" container_id running_image
  container_id="$(docker compose "${compose_args[@]}" ps -q "$SERVICE")"
  [ -n "$container_id" ] || { echo "No running container found for $SERVICE" >&2; return 1; }
  running_image="$(docker inspect --format='{{.Config.Image}}' "$container_id")"
  if [ "$running_image" != "$expected" ]; then
    echo "Exact-image verification failed: expected=$expected running=$running_image" >&2
    return 1
  fi
  echo "EXACT_IMAGE_VERIFIED=$running_image"
}

wait_until_healthy() {
  local label="$1" attempts="$2" delay="$3" attempt
  for ((attempt=1; attempt<=attempts; attempt++)); do
    if probe "$label $attempt/$attempts"; then
      return 0
    fi
    [ "$attempt" -lt "$attempts" ] && sleep "$delay"
  done
  return 1
}

monitor_sustained() {
  local phase attempts delay attempt
  for phase in short-term medium-term; do
    if [ "$phase" = short-term ]; then
      attempts="$SHORT_TERM_ATTEMPTS"
      delay="$SHORT_TERM_DELAY"
    else
      attempts="$MEDIUM_TERM_ATTEMPTS"
      delay="$MEDIUM_TERM_DELAY"
    fi
    for ((attempt=1; attempt<=attempts; attempt++)); do
      sleep "$delay"
      probe "$phase $attempt/$attempts" || return 1
    done
  done
}

rollback() {
  local status=$?
  trap - ERR
  echo "Deployment failed (status $status); restoring previous release" >&2
  if [ -n "$previous_image" ]; then
    set_image "$previous_image"
    if docker compose "${compose_args[@]}" config --quiet && \
       docker compose "${compose_args[@]}" pull "$SERVICE" && \
       docker compose "${compose_args[@]}" up -d --no-deps --force-recreate "$SERVICE" && \
       verify_running_image "$previous_image" && \
       wait_until_healthy "rollback" "$ROLLBACK_ATTEMPTS" "$ROLLBACK_DELAY"; then
      echo "ROLLBACK_RESULT=success"
      echo "Automatic rollback succeeded: $previous_image" >&2
    else
      echo "ROLLBACK_RESULT=failure"
      echo "Automatic rollback health or exact-image verification failed" >&2
    fi
  else
    cp "$env_backup" "$ENV_FILE"
    echo "ROLLBACK_RESULT=unavailable"
    echo "No previous image was discoverable; restored .env only" >&2
  fi
  exit "$status"
}
trap rollback ERR

set_image "$DEPLOY_IMAGE"
docker compose "${compose_args[@]}" config --quiet
docker compose "${compose_args[@]}" pull "$SERVICE"
docker compose "${compose_args[@]}" up -d --no-deps --force-recreate "$SERVICE"
verify_running_image "$DEPLOY_IMAGE"
wait_until_healthy "immediate" "$IMMEDIATE_ATTEMPTS" "$IMMEDIATE_DELAY"
monitor_sustained
trap - ERR
printf '%s\n' "$DEPLOY_IMAGE" > "$APPS_DIR/.${SERVICE}-last-successful-image"
echo "SUSTAINED_MONITORING=passed"
echo "Deployment healthy and sustained monitoring passed: $DEPLOY_IMAGE"
