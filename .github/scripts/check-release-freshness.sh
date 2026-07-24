#!/usr/bin/env bash
set -Eeuo pipefail

: "${ACTION:?ACTION is required}"
: "${RELEASE_SHA:?RELEASE_SHA is required}"
: "${CURRENT_MAIN_SHA:?CURRENT_MAIN_SHA is required}"

if [ "$ACTION" = rollback ]; then
  echo "Rollback freshness check skipped; the requested immutable rollback image was verified during preflight."
  exit 0
fi

sha_pattern='^[0-9a-f]{40}$'
if ! [[ "$RELEASE_SHA" =~ $sha_pattern ]]; then
  echo "Release SHA must be a full lowercase commit SHA." >&2
  exit 1
fi
if ! [[ "$CURRENT_MAIN_SHA" =~ $sha_pattern ]]; then
  echo "Current main SHA must be a full lowercase commit SHA." >&2
  exit 1
fi
if [ "$RELEASE_SHA" != "$CURRENT_MAIN_SHA" ]; then
  echo "Refusing superseded deployment: approved release is no longer current main." >&2
  echo "Approved release: $RELEASE_SHA" >&2
  echo "Current main:     $CURRENT_MAIN_SHA" >&2
  exit 1
fi

echo "Release freshness verified: approved release is current main."
