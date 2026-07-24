#!/usr/bin/env bash
set -Eeuo pipefail

: "${DEPLOY_IMAGE:?DEPLOY_IMAGE is required}"
ROLLBACK_IMAGE="${ROLLBACK_IMAGE:-}"
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
ROLLOUT_ID="${ROLLOUT_ID:-manual-$$}"
[[ "$ROLLOUT_ID" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "ROLLOUT_ID contains unsupported characters" >&2; exit 1; }
image_digest_pattern='^[^[:space:]@]+@sha256:[0-9a-f]{64}$'
[[ "$DEPLOY_IMAGE" =~ $image_digest_pattern ]] || { echo "DEPLOY_IMAGE must be an immutable repo@sha256 digest" >&2; exit 1; }
if [ -n "$ROLLBACK_IMAGE" ]; then
  [[ "$ROLLBACK_IMAGE" =~ $image_digest_pattern ]] || { echo "ROLLBACK_IMAGE must be an immutable repo@sha256 digest" >&2; exit 1; }
fi

cd "$APPS_DIR"
[ -f "$ENV_FILE" ] || touch "$ENV_FILE"
# GitHub concurrency is repository-scoped; serialize all repositories on the VPS.
exec 9>"$APPS_DIR/.production-deploy.lock"
flock -w 600 9 || { echo "Timed out waiting for the production deployment lock" >&2; exit 1; }
status_file="$APPS_DIR/.${SERVICE}-rollout-${ROLLOUT_ID}.status"

set_rollout_status() {
  local status="$1" image="${2:-}" tmp
  tmp="$(mktemp "${status_file}.XXXXXX")"
  printf 'ROLLOUT_STATUS=%s IMAGE=%s ROLLOUT_ID=%s\n' "$status" "$image" "$ROLLOUT_ID" > "$tmp"
  chmod 600 "$tmp"
  mv "$tmp" "$status_file"
}
set_rollout_status started "$DEPLOY_IMAGE"

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

container_id="$(docker compose "${compose_args[@]}" ps -q "$SERVICE" 2>/dev/null || true)"
previous_image=""

# Derive the rollback artifact from the image that is actually running. Successful
# workflow history can contain validation-only images that were never deployed.
if [ -n "$container_id" ]; then
  running_image_id="$(docker inspect --format='{{.Image}}' "$container_id")"
  release_repository="${DEPLOY_IMAGE%@sha256:*}"
  repo_digests="$(docker image inspect --format='{{join .RepoDigests ","}}' "$running_image_id")"
  IFS=',' read -r -a digest_candidates <<< "$repo_digests"
  for candidate in "${digest_candidates[@]}"; do
    if [[ "$candidate" == "$release_repository"@sha256:* ]]; then
      previous_image="$candidate"
      break
    fi
  done
  if [ -z "$previous_image" ]; then
    set_rollout_status rollback_image_unavailable "$running_image_id"
    echo "DEPLOY_ABORTED=running-release-has-no-immutable-registry-digest" >&2
    exit 1
  fi
  if [ -n "$ROLLBACK_IMAGE" ] && [ "$ROLLBACK_IMAGE" != "$previous_image" ]; then
    echo "Ignoring stale CI rollback candidate; using the running release digest" >&2
  fi
  echo "ROLLBACK_IMAGE_RESOLVED=$previous_image"
fi

# Fail before touching production when the running rollback artifact is unavailable.
if [ -n "$previous_image" ]; then
  echo "Verifying rollback image is pullable: $previous_image"
  if ! docker pull "$previous_image" >/dev/null; then
    set_rollout_status rollback_image_unavailable "$previous_image"
    echo "DEPLOY_ABORTED=rollback-image-unavailable" >&2
    exit 1
  fi
  rollback_image_id="$(docker image inspect --format='{{.Id}}' "$previous_image")"
  if [ "$running_image_id" != "$rollback_image_id" ]; then
    set_rollout_status rollback_image_mismatch "$previous_image"
    echo "DEPLOY_ABORTED=resolved-rollback-image-does-not-match-running-release" >&2
    exit 1
  fi
  echo "ROLLBACK_WORTHINESS=verified"
else
  echo "No running release was discoverable; this is a first deployment without an image rollback path" >&2
  echo "ROLLBACK_WORTHINESS=unavailable"
fi
set_rollout_status preflight_passed "$previous_image"

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

production_mutated=false
rollback() {
  local status=$?
  trap - ERR
  set_rollout_status rollback_started "$previous_image"
  echo "Deployment failed (status $status); restoring previous release" >&2
  if [ -n "$previous_image" ]; then
    set_image "$previous_image"
    if docker compose "${compose_args[@]}" config --quiet && \
       docker compose "${compose_args[@]}" pull "$SERVICE" && \
       docker compose "${compose_args[@]}" up -d --no-deps --force-recreate "$SERVICE" && \
       verify_running_image "$previous_image" && \
       wait_until_healthy "rollback" "$ROLLBACK_ATTEMPTS" "$ROLLBACK_DELAY"; then
      set_rollout_status rollback_success "$previous_image"
      echo "ROLLBACK_RESULT=success"
      echo "Automatic rollback succeeded: $previous_image" >&2
    else
      set_rollout_status rollback_failure "$previous_image"
      echo "ROLLBACK_RESULT=failure"
      echo "Automatic rollback health or exact-image verification failed" >&2
    fi
  else
    cp "$env_backup" "$ENV_FILE"
    if [ "$production_mutated" = true ] && docker compose "${compose_args[@]}" rm --stop --force "$SERVICE"; then
      set_rollout_status rollback_first_deploy_removed ""
      echo "ROLLBACK_RESULT=first-deploy-removed"
      echo "Failed first-deploy service container removed; previous .env restored" >&2
    elif [ "$production_mutated" = true ]; then
      set_rollout_status rollback_failure ""
      echo "ROLLBACK_RESULT=failure"
      echo "Failed first-deploy service container could not be removed" >&2
    else
      set_rollout_status rollback_unavailable ""
      echo "ROLLBACK_RESULT=unavailable"
      echo "No production mutation occurred; previous .env restored" >&2
    fi
  fi
  exit "$status"
}
trap rollback ERR

set_rollout_status deploying "$DEPLOY_IMAGE"
set_image "$DEPLOY_IMAGE"
docker compose "${compose_args[@]}" config --quiet
docker compose "${compose_args[@]}" pull "$SERVICE"
production_mutated=true
docker compose "${compose_args[@]}" up -d --no-deps --force-recreate "$SERVICE"
verify_running_image "$DEPLOY_IMAGE"
set_rollout_status image_verified "$DEPLOY_IMAGE"
wait_until_healthy "immediate" "$IMMEDIATE_ATTEMPTS" "$IMMEDIATE_DELAY"
set_rollout_status immediate_passed "$DEPLOY_IMAGE"
set_rollout_status sustained_monitoring "$DEPLOY_IMAGE"
monitor_sustained
trap - ERR
printf '%s\n' "$DEPLOY_IMAGE" > "$APPS_DIR/.${SERVICE}-last-successful-image"
set_rollout_status success "$DEPLOY_IMAGE"
echo "SUSTAINED_MONITORING=passed"
echo "Deployment healthy and sustained monitoring passed: $DEPLOY_IMAGE"
