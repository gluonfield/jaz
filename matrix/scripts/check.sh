#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

. ./scripts/lib.sh

if [ -z "${JAZ_RUNTIME_ROOT:-}" ] && [ -z "${JAZ_MATRIX_ROOT:-}" ]; then
	tmp_root="$(mktemp -d)"
	trap 'rm -rf "$tmp_root"' EXIT
	export JAZ_RUNTIME_ROOT="$tmp_root"
fi

matrix_root="$(matrix_root)"
mkdir -p "$matrix_root/secrets"
if [ ! -f "$matrix_root/secrets/postgres_password" ]; then
	printf "check-only\n" > "$matrix_root/secrets/postgres_password"
fi

JAZ_MATRIX_LOCAL=1 ./scripts/compose.sh config >/dev/null
echo "matrix compose config ok"
