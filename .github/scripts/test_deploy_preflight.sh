#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"

cat > "$tmp/bin/curl" <<'SH'
#!/usr/bin/env bash
printf '401'
SH
cat > "$tmp/bin/gh" <<'SH'
#!/usr/bin/env bash
[ "${MOCK_GH_FAIL:-0}" != 1 ] || exit 42
printf '%s\n' \
  bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  cccccccccccccccccccccccccccccccccccccccc
SH
cat > "$tmp/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = login ]; then exit 0; fi
if [ "${1:-}" = manifest ] && [ "${2:-}" = inspect ] && [ "${3:-}" = --verbose ]; then
  case "${4:-}" in
    ghcr.io/example/backend:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa)
      printf '{"Descriptor":{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","platform":{"os":"linux","architecture":"amd64"}}}' ;;
    ghcr.io/example/backend:cccccccccccccccccccccccccccccccccccccccc)
      printf '{"Descriptor":{"digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","platform":{"os":"linux","architecture":"amd64"}}}' ;;
    *) exit 1 ;;
  esac
  exit 0
fi
exit 1
SH
chmod +x "$tmp/bin/"*

output="$tmp/output"
summary="$tmp/summary"
(
  cd "$repo_root"
  PATH="$tmp/bin:$PATH" \
  IMAGE_REPOSITORY=ghcr.io/example/backend \
  DEPLOY_HEALTH_URL=https://example.com/health \
  DEPLOY_COMPOSE_FILES=/opt/apps/compose.yml \
  DEPLOY_SERVICE=backend \
  DEPLOY_IMAGE_VARIABLE=BACKEND_IMAGE \
  GH_TOKEN=test GITHUB_ACTOR=test GITHUB_REPOSITORY=example/backend \
  GITHUB_OUTPUT="$output" GITHUB_STEP_SUMMARY="$summary" \
  ACTION=rollback REQUESTED_IMAGE_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  bash .github/scripts/deploy-preflight.sh
)

grep -qx 'release_image=ghcr.io/example/backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' "$output"
grep -qx 'previous_image=ghcr.io/example/backend@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc' "$output"
grep -qx 'rollback_worthy=true' "$output"
if (
  cd "$repo_root"
  PATH="$tmp/bin:$PATH" MOCK_GH_FAIL=1 \
  IMAGE_REPOSITORY=ghcr.io/example/backend DEPLOY_HEALTH_URL=https://example.com/health \
  DEPLOY_COMPOSE_FILES=/opt/apps/compose.yml DEPLOY_SERVICE=backend DEPLOY_IMAGE_VARIABLE=BACKEND_IMAGE \
  GH_TOKEN=test GITHUB_ACTOR=test GITHUB_REPOSITORY=example/backend \
  GITHUB_OUTPUT="$tmp/failed-output" GITHUB_STEP_SUMMARY="$tmp/failed-summary" \
  ACTION=rollback REQUESTED_IMAGE_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  bash .github/scripts/deploy-preflight.sh >/dev/null 2>&1
); then
  echo "deploy preflight unexpectedly ignored GitHub API failure" >&2
  exit 1
fi
grep -Fq 'manifest_sha="$GITHUB_SHA"' .github/workflows/ci.yml
grep -Fq 'git show "$manifest_sha:migrations/manifest.sha256"' .github/workflows/ci.yml
grep -Fq 'ROLLBACK_IMAGE: ${{ needs.deploy-preflight.outputs.previous_image }}' .github/workflows/ci.yml
grep -Fq 'rollback_image_id="$(docker image inspect' deploy/deploy-compose-service.sh
if DEPLOY_IMAGE=ghcr.io/example/backend:mutable ROLLBACK_IMAGE='' IMAGE_VARIABLE=BACKEND_IMAGE \
  SERVICE=backend HEALTH_URL=https://example.com/health GHCR_USERNAME=test GHCR_TOKEN=test \
  bash deploy/deploy-compose-service.sh >/dev/null 2>&1; then
  echo "deploy script unexpectedly accepted a mutable release tag" >&2
  exit 1
fi
printf 'deploy preflight image-selection and rollback-manifest tests passed\n'
