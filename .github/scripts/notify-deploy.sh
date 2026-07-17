#!/usr/bin/env bash
set -Eeuo pipefail

event="${1:?Usage: notify-deploy.sh <start|success|failure>}"
webhook="${DISCORD_WEBHOOK_URL:-}"
deploy_image="${DEPLOY_IMAGE:-unknown}"
previous_image="${PREVIOUS_IMAGE:-unknown}"
log_file="${DEPLOY_LOG_FILE:-}"
rollback="not-triggered"

if [ "$event" = failure ] && [ -n "$log_file" ] && [ -r "$log_file" ]; then
  if grep -q 'ROLLBACK_RESULT=success' "$log_file"; then rollback="succeeded";
  elif grep -q 'ROLLBACK_RESULT=failure' "$log_file"; then rollback="failed";
  elif grep -q 'ROLLBACK_RESULT=unavailable' "$log_file"; then rollback="unavailable";
  else rollback="unknown";
  fi
fi

case "$event" in
  start) title="Deployment starting" ;;
  success) title="Deployment succeeded" ;;
  failure) title="Deployment failed; rollback ${rollback}" ;;
  *) echo "Unsupported notification event: $event" >&2; exit 2 ;;
esac

{
  echo "## $title"
  echo
  echo "- Commit: \`${GITHUB_SHA}\`"
  echo "- Release image: \`$deploy_image\`"
  echo "- Previous image: \`$previous_image\`"
  echo "- Run: ${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}"
} >> "$GITHUB_STEP_SUMMARY"

if [ -z "$webhook" ]; then
  echo "DISCORD_WEBHOOK_URL is not configured; GitHub summary was written and webhook delivery was skipped."
  exit 0
fi

payload="$(EVENT_TITLE="$title" ROLLBACK_RESULT="$rollback" python3 - <<'PY'
import json
import os

print(json.dumps({
    "content": (
        f"**{os.environ['EVENT_TITLE']}**\n"
        f"Commit: `{os.environ['GITHUB_SHA'][:7]}`\n"
        f"Release: `{os.environ.get('DEPLOY_IMAGE', 'unknown')}`\n"
        f"Previous: `{os.environ.get('PREVIOUS_IMAGE', 'unknown')}`\n"
        f"Rollback: `{os.environ['ROLLBACK_RESULT']}`\n"
        f"Run: {os.environ['GITHUB_SERVER_URL']}/{os.environ['GITHUB_REPOSITORY']}/actions/runs/{os.environ['GITHUB_RUN_ID']}"
    )
}))
PY
)"

if ! curl --fail --silent --show-error --max-time 15 \
  --header 'Content-Type: application/json' \
  --data "$payload" "$webhook"; then
  echo "Discord notification failed; deployment result is unchanged" >&2
fi
