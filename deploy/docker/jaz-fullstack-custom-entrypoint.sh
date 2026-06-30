#!/usr/bin/env bash
set -euo pipefail

export PORT="${PORT:-8080}"
export JAZ_BACKEND_PORT="${JAZ_BACKEND_PORT:-5299}"

( PORT="${JAZ_BACKEND_PORT}" /usr/local/bin/jaz-backend-entrypoint.sh ) &
backend_pid=$!

caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
caddy_pid=$!

shutdown() {
	kill "${backend_pid}" "${caddy_pid}" 2>/dev/null || true
}
trap shutdown INT TERM

set +e
wait -n "${backend_pid}" "${caddy_pid}"
status=$?
shutdown
wait "${backend_pid}" "${caddy_pid}" 2>/dev/null
exit "${status}"
