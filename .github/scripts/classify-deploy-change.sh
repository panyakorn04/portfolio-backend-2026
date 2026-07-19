#!/usr/bin/env bash
set -Eeuo pipefail

classify() {
  grep -Eq '^(Dockerfile|\.dockerignore|docker-compose\.ya?ml|go\.mod|go\.sum|main\.go|cmd/|internal/|etc/|ai/|deploy/|migrations/)'
}

case "${1:-classify}" in
  classify)
    if classify; then
      printf 'true\n'
    else
      printf 'false\n'
    fi
    ;;
  --self-test)
    test "$(printf '%s\n' README.md .github/workflows/ci.yml | "$0")" = false
    test "$(printf '%s\n' internal/handler/public.go | "$0")" = true
    test "$(printf '%s\n' migrations/0020_example.sql | "$0")" = true
    test "$(printf '%s\n' Dockerfile | "$0")" = true
    printf 'deploy change classifier tests passed\n'
    ;;
  *)
    echo "Usage: $0 [classify|--self-test]" >&2
    exit 2
    ;;
esac
