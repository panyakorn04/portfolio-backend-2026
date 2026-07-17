#!/usr/bin/env bash
set -Eeuo pipefail

: "${IMAGE_REPOSITORY:?IMAGE_REPOSITORY is required}"
: "${DEPLOY_HEALTH_URL:?DEPLOY_HEALTH_URL is required}"
: "${DEPLOY_COMPOSE_FILES:?DEPLOY_COMPOSE_FILES is required}"
: "${DEPLOY_SERVICE:?DEPLOY_SERVICE is required}"
: "${DEPLOY_IMAGE_VARIABLE:?DEPLOY_IMAGE_VARIABLE is required}"
: "${GH_TOKEN:?GH_TOKEN is required}"
: "${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
: "${GITHUB_STEP_SUMMARY:?GITHUB_STEP_SUMMARY is required}"

ACTION="${ACTION:-deploy}"
DEPLOY_TIMEZONE="${DEPLOY_TIMEZONE:-Asia/Bangkok}"
EMERGENCY_OVERRIDE="${EMERGENCY_OVERRIDE:-false}"
MIGRATION_VERIFIED="${MIGRATION_VERIFIED:-false}"
BEFORE_SHA="${BEFORE_SHA:-}"
REQUESTED_IMAGE_SHA="${REQUESTED_IMAGE_SHA:-}"

if [ "$ACTION" = rollback ]; then
  RELEASE_SHA="$REQUESTED_IMAGE_SHA"
else
  RELEASE_SHA="${RELEASE_SHA:-$GITHUB_SHA}"
fi
[[ "$RELEASE_SHA" =~ ^[0-9a-f]{40}$ ]] || { echo "Release SHA must be a full lowercase 40-character commit SHA" >&2; exit 1; }

required_assets=(
  Dockerfile
  .dockerignore
  deploy/deploy-compose-service.sh
  .github/scripts/smoke-test-image.sh
  .github/scripts/reconcile-rollout.sh
  .github/scripts/notify-deploy.sh
)
for asset in "${required_assets[@]}"; do
  [ -r "$asset" ] || { echo "Missing required deployment asset: $asset" >&2; exit 1; }
done

echo "Checking deployment window in $DEPLOY_TIMEZONE"
window_ok=true
if [ "$ACTION" != rollback ] && [ "$EMERGENCY_OVERRIDE" != true ]; then
  day="$(TZ="$DEPLOY_TIMEZONE" date +%u)"
  hour="$(TZ="$DEPLOY_TIMEZONE" date +%H)"
  if [ "$day" -eq 5 ] && [ "$hour" -ge 15 ]; then window_ok=false; fi
  if [ "$day" -eq 6 ] || [ "$day" -eq 7 ]; then window_ok=false; fi
  if [ "$day" -eq 1 ] && [ "$hour" -lt 8 ]; then window_ok=false; fi
fi
if [ "$window_ok" != true ]; then
  echo "Deployment blocked outside the safe window; use an approved workflow_dispatch emergency override if required" >&2
  exit 1
fi

http_code="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 https://ghcr.io/v2/ || true)"
[ -n "$http_code" ] && [ "$http_code" != 000 ] || { echo "GHCR is unreachable" >&2; exit 1; }
echo "GHCR reachable (HTTP $http_code)"

printf '%s\n' "$GH_TOKEN" | docker login ghcr.io --username "$GITHUB_ACTOR" --password-stdin >/dev/null
current_image="${IMAGE_REPOSITORY}:${RELEASE_SHA}"
docker manifest inspect "$current_image" >/dev/null
echo "Release image exists: $current_image"

previous_sha=""
while IFS= read -r candidate; do
  if [ -n "$candidate" ] && [ "$candidate" != "$RELEASE_SHA" ]; then
    previous_sha="$candidate"
    break
  fi
done < <(gh api "repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?per_page=20&status=success&event=push" --jq '.workflow_runs[].head_sha')

previous_image=""
rollback_worthy="first-deploy"
if [ -n "$previous_sha" ]; then
  previous_image="${IMAGE_REPOSITORY}:${previous_sha}"
  if docker manifest inspect "$previous_image" >/dev/null 2>&1; then
    rollback_worthy=true
    echo "Rollback image exists: $previous_image"
  else
    rollback_worthy=false
    echo "Previous successful image is not pullable: $previous_image" >&2
    exit 1
  fi
fi

migrations=""
if [ "$ACTION" != rollback ]; then
  base_sha="$BEFORE_SHA"
  if ! [[ "$base_sha" =~ ^[0-9a-f]{40}$ ]] || [[ "$base_sha" =~ ^0+$ ]] || ! git cat-file -e "${base_sha}^{commit}" 2>/dev/null; then
    base_sha="$previous_sha"
  fi
  if [ -z "$base_sha" ]; then
    base_sha="$(git hash-object -t tree /dev/null)"
  fi
  migrations="$(git diff --name-only "$base_sha" "$GITHUB_SHA" -- 'migrations/*.sql' | paste -sd, -)"
  if [ -n "$migrations" ] && [ "$MIGRATION_VERIFIED" != true ]; then
    {
      echo "## Production rollout preflight: NO-GO"
      echo
      echo "Production migrations require manual application and verification before deployment:"
      echo
      echo "\`$migrations\`"
      echo
      echo "Apply and verify them, then run workflow_dispatch with migration_verified enabled."
    } >> "$GITHUB_STEP_SUMMARY"
    echo "Unverified production migrations: $migrations" >&2
    exit 1
  fi
fi

{
  echo "release_image=$current_image"
  echo "previous_image=$previous_image"
  echo "rollback_worthy=$rollback_worthy"
  echo "migrations=${migrations:-none}"
  echo "window_ok=$window_ok"
} >> "$GITHUB_OUTPUT"

{
  echo "## Production rollout preflight: GO"
  echo
  echo "| Check | Result |"
  echo "|---|---|"
  echo "| Deployment window | $window_ok ($DEPLOY_TIMEZONE) |"
  echo "| Release image | \`$current_image\` |"
  echo "| Rollback image | \`${previous_image:-first deployment}\` |"
  echo "| Rollback worthy | $rollback_worthy |"
  echo "| Migrations | \`${migrations:-none}\` |"
  echo "| Health target | $DEPLOY_HEALTH_URL |"
} >> "$GITHUB_STEP_SUMMARY"
