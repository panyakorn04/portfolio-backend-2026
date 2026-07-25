#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/check-release-freshness.sh"

run_case() {
  local expected_status="$1"
  shift
  set +e
  output="$(env "$@" bash "$checker" 2>&1)"
  status=$?
  set -e
  if [ "$status" -ne "$expected_status" ]; then
    printf 'expected status %s, got %s: %s\n' "$expected_status" "$status" "$output" >&2
    exit 1
  fi
}

sha_a="1111111111111111111111111111111111111111"
sha_b="2222222222222222222222222222222222222222"

run_case 0 ACTION=rollback RELEASE_SHA="$sha_a" CURRENT_MAIN_SHA="$sha_b"
run_case 0 ACTION=deploy RELEASE_SHA="$sha_a" CURRENT_MAIN_SHA="$sha_a"
run_case 1 ACTION=deploy RELEASE_SHA="$sha_a" CURRENT_MAIN_SHA="$sha_b"
run_case 1 ACTION=deploy RELEASE_SHA=short CURRENT_MAIN_SHA="$sha_a"
run_case 1 ACTION=deploy RELEASE_SHA="$sha_a" CURRENT_MAIN_SHA=short

workflow="$script_dir/../workflows/ci.yml"
[ "$(grep -c 'bash .github/scripts/check-release-freshness.sh' "$workflow")" -eq 2 ]
recheck_line="$(grep -n 'Recheck release freshness before production mutation' "$workflow" | cut -d: -f1)"
notify_line="$(grep -n 'Notify deployment start' "$workflow" | cut -d: -f1)"
[ "$recheck_line" -lt "$notify_line" ]

printf 'release freshness tests passed\n'
