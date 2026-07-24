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
BEFORE_SHA="${BEFORE_SHA:-}"
REQUESTED_IMAGE_SHA="${REQUESTED_IMAGE_SHA:-}"

resolve_image_ref() {
  local tagged_ref="$1"
  local manifest_json digest
  manifest_json="$(docker manifest inspect --verbose "$tagged_ref")" || return 1
  digest="$(jq -r '
    if type == "array" then
      ([.[] | select(.Descriptor.platform.os == "linux" and .Descriptor.platform.architecture == "amd64") | .Descriptor.digest][0] // empty)
    else
      (.Descriptor.digest // .digest // empty)
    end
  ' <<<"$manifest_json")"
  [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || return 1
  printf '%s@%s\n' "${tagged_ref%:*}" "$digest"
}

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

http_code="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 https://ghcr.io/v2/ || true)"
[ -n "$http_code" ] && [ "$http_code" != 000 ] || { echo "GHCR is unreachable" >&2; exit 1; }
echo "GHCR reachable (HTTP $http_code)"

printf '%s\n' "$GH_TOKEN" | docker login ghcr.io --username "$GITHUB_ACTOR" --password-stdin >/dev/null
current_tag="${IMAGE_REPOSITORY}:${RELEASE_SHA}"
current_image="$(resolve_image_ref "$current_tag")" || {
  echo "Release image is unavailable or has no immutable digest: $current_tag" >&2
  exit 1
}
echo "Release image exists: $current_image"

previous_sha=""
previous_image=""
successful_candidates=0
candidate_shas="$(gh api "repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?per_page=20&status=success&event=push" --jq '.workflow_runs[].head_sha')" || {
  echo "Unable to enumerate successful deployment candidates" >&2
  exit 1
}
while IFS= read -r candidate; do
  if [ -z "$candidate" ] || [ "$candidate" = "$RELEASE_SHA" ]; then
    continue
  fi
  successful_candidates=$((successful_candidates + 1))
  candidate_tag="${IMAGE_REPOSITORY}:${candidate}"
  if candidate_image="$(resolve_image_ref "$candidate_tag" 2>/dev/null)"; then
    previous_sha="$candidate"
    previous_image="$candidate_image"
    break
  fi
  echo "Skipping successful validation-only run without a pullable image: $candidate_tag"
done <<< "$candidate_shas"

rollback_worthy="first-deploy"
if [ -n "$previous_sha" ]; then
  rollback_worthy=true
  echo "Rollback image exists: $previous_image"
elif [ "$successful_candidates" -gt 0 ]; then
  rollback_worthy=false
  echo "No pullable rollback image was found among the last $successful_candidates successful push runs" >&2
  exit 1
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
fi

{
  echo "release_image=$current_image"
  echo "previous_image=$previous_image"
  echo "rollback_worthy=$rollback_worthy"
  echo "migrations=${migrations:-none}"
} >> "$GITHUB_OUTPUT"

{
  echo "## Production rollout preflight: ready for approval"
  echo
  echo "| Check | Result |"
  echo "|---|---|"
  echo "| Release image | \`$current_image\` |"
  echo "| Rollback image | \`${previous_image:-first deployment}\` |"
  echo "| Rollback worthy | $rollback_worthy |"
  echo "| Migrations | \`${migrations:-none}\` |"
  echo "| Health target | $DEPLOY_HEALTH_URL |"
  echo
  echo "Production remains unchanged until a required reviewer approves the protected \`production\` environment."
} >> "$GITHUB_STEP_SUMMARY"
