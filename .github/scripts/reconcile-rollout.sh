#!/usr/bin/env bash
set -Eeuo pipefail

: "${VPS_HOST:?VPS_HOST is required}"
: "${VPS_USER:?VPS_USER is required}"
: "${DEPLOY_SERVICE:?DEPLOY_SERVICE is required}"
: "${DEPLOY_IMAGE:?DEPLOY_IMAGE is required}"
: "${ROLLOUT_ID:?ROLLOUT_ID is required}"

status_file="/opt/apps/.${DEPLOY_SERVICE}-rollout-${ROLLOUT_ID}.status"
remote_env="$(printf 'STATUS_FILE=%q SERVICE=%q EXPECTED_IMAGE=%q' "$status_file" "$DEPLOY_SERVICE" "$DEPLOY_IMAGE")"

set +e
result="$(ssh -i ~/.ssh/deploy_key \
  -o BatchMode=yes \
  -o StrictHostKeyChecking=yes \
  -o ConnectTimeout=30 \
  -o ServerAliveInterval=10 \
  -o ServerAliveCountMax=2 \
  "${VPS_USER}@${VPS_HOST}" \
  "${remote_env} bash -se" <<'REMOTE'
set -euo pipefail
if [ -r "$STATUS_FILE" ]; then
  status_line="$(cat "$STATUS_FILE")"
else
  status_line="ROLLOUT_STATUS=missing IMAGE= ROLLOUT_ID="
fi
container_ids="$(docker ps --filter "label=com.docker.compose.service=$SERVICE" --format '{{.ID}}')"
container_id="${container_ids%%$'\n'*}"
running_image=""
if [ -n "$container_id" ]; then
  running_image="$(docker inspect --format='{{.Config.Image}}' "$container_id" 2>/dev/null || true)"
fi
printf '%s RUNNING_IMAGE=%s\n' "$status_line" "$running_image"
REMOTE
)"
ssh_status=$?
set -e

if [ "$ssh_status" -ne 0 ]; then
  echo "Reconciliation SSH failed with status $ssh_status; refusing an ambiguous retry" >&2
  exit 21
fi

echo "$result"
rollout_status="$(printf '%s\n' "$result" | awk '{for (i=1; i<=NF; i++) if ($i ~ /^ROLLOUT_STATUS=/) {sub(/^ROLLOUT_STATUS=/, "", $i); print $i; exit}}')"
running_image="$(printf '%s\n' "$result" | awk '{for (i=1; i<=NF; i++) if ($i ~ /^RUNNING_IMAGE=/) {sub(/^RUNNING_IMAGE=/, "", $i); print $i; exit}}')"

case "$rollout_status" in
  success)
    if [ "$running_image" = "$DEPLOY_IMAGE" ]; then
      echo "RECONCILE_DECISION=success"
      exit 0
    fi
    echo "Rollout recorded success but exact running image differs" >&2
    exit 21
    ;;
  missing)
    echo "RECONCILE_DECISION=retry"
    exit 10
    ;;
  rollback_success|rollback_failure|rollback_unavailable|rollback_image_unavailable)
    echo "RECONCILE_DECISION=failed"
    exit 20
    ;;
  started|preflight_passed|deploying|image_verified|immediate_passed|sustained_monitoring|rollback_started)
    echo "Rollout is in an ambiguous non-terminal state ($rollout_status); refusing to replay it" >&2
    exit 21
    ;;
  *)
    echo "Unknown rollout state ($rollout_status); refusing to replay it" >&2
    exit 21
    ;;
esac
