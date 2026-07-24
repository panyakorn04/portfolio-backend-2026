#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/apps"
touch "$tmp/apps/docker-compose.yml"
printf 'BACKEND_IMAGE=ghcr.io/example/backend:old\n' > "$tmp/apps/.env"

cat > "$tmp/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = login ] || [ "$1" = pull ]; then exit 0; fi
if [ "$1" = compose ]; then
  case " $* " in
    *" ps -q backend "*) printf 'container-id\n' ;;
    *" config --quiet "*|*" pull backend "*|*" up -d "*) exit 0 ;;
    *) exit 0 ;;
  esac
  exit 0
fi
if [ "$1" = inspect ]; then
  case "$2" in
    --format='{{.Image}}') printf 'sha256:running-image-id\n' ;;
    --format='{{.Config.Image}}') printf '%s\n' "$DEPLOY_IMAGE" ;;
    *) exit 1 ;;
  esac
  exit 0
fi
if [ "$1" = image ] && [ "$2" = inspect ]; then
  format="$3"
  image="$4"
  if [ "$format" = "--format={{join .RepoDigests \",\"}}" ]; then
    printf 'ghcr.io/example/backend@sha256:%064d\n' 3
  elif [ "$format" = "--format={{.Id}}" ]; then
    case "$image" in
      *@sha256:*3) printf 'sha256:running-image-id\n' ;;
      *) printf 'sha256:stale-image-id\n' ;;
    esac
  fi
  exit 0
fi
exit 1
SH
chmod +x "$tmp/bin/docker"
cat > "$tmp/bin/curl" <<'SH'
#!/usr/bin/env bash
printf '200 0.01'
SH
chmod +x "$tmp/bin/curl"
cat > "$tmp/bin/flock" <<'SH'
#!/usr/bin/env bash
exit 0
SH
chmod +x "$tmp/bin/flock"

actual="ghcr.io/example/backend@sha256:$(printf '%064d' 3)"
stale="ghcr.io/example/backend@sha256:$(printf '%064d' 2)"
release="ghcr.io/example/backend@sha256:$(printf '%064d' 1)"
output="$tmp/output.log"
set +e
PATH="$tmp/bin:$PATH" \
APPS_DIR="$tmp/apps" ENV_FILE="$tmp/apps/.env" COMPOSE_FILES=docker-compose.yml \
DEPLOY_IMAGE="$release" ROLLBACK_IMAGE="$stale" IMAGE_VARIABLE=BACKEND_IMAGE SERVICE=backend \
HEALTH_URL=https://example.com/health GHCR_USERNAME=test GHCR_TOKEN=test ROLLOUT_ID=test \
IMMEDIATE_ATTEMPTS=1 IMMEDIATE_DELAY=0 SHORT_TERM_ATTEMPTS=1 SHORT_TERM_DELAY=0 \
MEDIUM_TERM_ATTEMPTS=1 MEDIUM_TERM_DELAY=0 ROLLBACK_ATTEMPTS=1 ROLLBACK_DELAY=0 \
bash "$repo_root/deploy/deploy-compose-service.sh" >"$output" 2>&1
status=$?
set -e
if [ "$status" -ne 0 ]; then
  while IFS= read -r line; do printf '%s\n' "$line" >&2; done < "$output"
  exit "$status"
fi

for expected in "ROLLBACK_IMAGE_RESOLVED=$actual" "SUSTAINED_MONITORING=passed"; do
  if ! grep -Fq "$expected" "$output"; then
    while IFS= read -r line; do printf '%s\n' "$line" >&2; done < "$output"
    echo "missing expected output: $expected" >&2
    exit 1
  fi
done
grep -Fqx "BACKEND_IMAGE=$release" "$tmp/apps/.env"
printf 'live rollback digest resolution test passed\n'
