#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."
. ./scripts/lib.sh

export JAZ_RUNTIME_ROOT="${JAZ_RUNTIME_ROOT:-$(runtime_root)}"
export JAZ_MATRIX_ROOT="${JAZ_MATRIX_ROOT:-$(matrix_root)}"

if [ -z "${SYNAPSE_SERVER_NAME:-}" ]; then
	server_name="$(configured_server_name || true)"
	if [ -z "$server_name" ] && [ "${JAZ_MATRIX_LOCAL:-}" = "1" ]; then
		server_name="localhost"
	fi
	if [ -n "$server_name" ]; then
		export SYNAPSE_SERVER_NAME="$server_name"
	fi
fi

compose -f docker-compose.yaml "$@"
