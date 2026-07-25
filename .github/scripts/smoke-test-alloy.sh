#!/usr/bin/env bash
set -Eeuo pipefail

compose_file="${1:-observability/alloy/docker-compose.yml}"
export GRAFANA_LOKI_URL="${GRAFANA_LOKI_URL:-https://example.com/loki/api/v1/push}"
export GRAFANA_LOKI_USERNAME="${GRAFANA_LOKI_USERNAME:-alloy-smoke-test}"
export GRAFANA_LOKI_TOKEN="${GRAFANA_LOKI_TOKEN:-alloy-smoke-test}"

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose -f "$compose_file")
elif command -v docker-compose >/dev/null 2>&1; then
  compose=(docker-compose -f "$compose_file")
else
  echo "Docker Compose is required" >&2
  exit 1
fi
cleanup() {
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${compose[@]}" config --quiet
"${compose[@]}" up -d --force-recreate --remove-orphans

for _ in $(seq 1 30); do
  if curl --fail --silent --show-error --max-time 2 http://127.0.0.1:12345/-/ready >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl --fail --silent --show-error --max-time 2 http://127.0.0.1:12345/-/ready >/dev/null

alloy_id="$("${compose[@]}" ps -q grafana-alloy)"
proxy_id="$("${compose[@]}" ps -q docker-socket-proxy)"
test -n "$alloy_id"
test -n "$proxy_id"

python3 - "$alloy_id" "$proxy_id" <<'PY'
import json
import subprocess
import sys

alloy = json.loads(subprocess.check_output(["docker", "inspect", sys.argv[1]]))[0]
proxy = json.loads(subprocess.check_output(["docker", "inspect", sys.argv[2]]))[0]
for name, container in (("alloy", alloy), ("proxy", proxy)):
    host = container["HostConfig"]
    if not host.get("ReadonlyRootfs"):
        raise SystemExit(f"{name} root filesystem is writable")
    if "ALL" not in (host.get("CapDrop") or []):
        raise SystemExit(f"{name} does not drop all capabilities")
if any(mount.get("Destination") == "/var/run/docker.sock" for mount in alloy.get("Mounts", [])):
    raise SystemExit("Alloy directly mounts the Docker socket")
socket_mounts = [m for m in proxy.get("Mounts", []) if m.get("Destination") == "/var/run/docker.sock"]
if len(socket_mounts) != 1 or socket_mounts[0].get("RW"):
    raise SystemExit("proxy Docker socket mount must be uniquely read-only")
PY

private_network=""
for network in $(docker inspect "$proxy_id" --format '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}'); do
  if [ "$(docker network inspect "$network" --format '{{.Internal}}')" = true ]; then
    private_network="$network"
    break
  fi
done
test -n "$private_network"

curl_image='curlimages/curl:8.16.0@sha256:463eaf6072688fe96ac64fa623fe73e1dbe25d8ad6c34404a669ad3ce1f104b6'
docker run --rm --network "$private_network" "$curl_image" \
  --fail --silent --show-error http://docker-socket-proxy:2375/_ping >/dev/null
docker run --rm --network "$private_network" "$curl_image" \
  --fail --silent --show-error http://docker-socket-proxy:2375/containers/json >/dev/null
docker run --rm --network "$private_network" "$curl_image" \
  --fail --silent --show-error http://docker-socket-proxy:2375/networks >/dev/null
post_status="$(docker run --rm --network "$private_network" "$curl_image" \
  --silent --output /dev/null --write-out '%{http_code}' --request POST \
  http://docker-socket-proxy:2375/containers/create)"
test "$post_status" = 403

if "${compose[@]}" logs --no-color grafana-alloy 2>&1 | python3 -c \
  'import sys; raise SystemExit(0 if any("Unable to refresh target groups" in line for line in sys.stdin) else 1)'; then
  echo "Alloy Docker discovery reported refresh errors" >&2
  exit 1
fi

echo "Alloy smoke test passed: ready=200 proxy_gets=ok proxy_post=403"
