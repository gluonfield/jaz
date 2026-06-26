#!/usr/bin/env bash
set -euo pipefail

version="$1"
ldflags="-s -w -X main.version=${version}"
telegram_id="${JAZ_BUNDLED_TELEGRAM_APP_ID:-}"
telegram_hash="${JAZ_BUNDLED_TELEGRAM_APP_HASH:-}"

if [ -n "${telegram_id}${telegram_hash}" ]; then
  if [ -z "$telegram_id" ] || [ -z "$telegram_hash" ]; then
    echo "JAZ_BUNDLED_TELEGRAM_APP_ID and JAZ_BUNDLED_TELEGRAM_APP_HASH must both be set" >&2
    exit 1
  fi
  case "$telegram_id" in
    *[!0-9]*)
      echo "JAZ_BUNDLED_TELEGRAM_APP_ID must be numeric" >&2
      exit 1
      ;;
  esac
  ldflags="${ldflags} -X github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientID=${telegram_id} -X github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientHash=${telegram_hash}"
fi

printf '%s\n' "$ldflags"
