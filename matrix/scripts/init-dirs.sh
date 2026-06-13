#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

. ./scripts/lib.sh

matrix_root="$(matrix_root)"

mkdir -p "$matrix_root/postgres"
mkdir -p "$matrix_root/secrets"
mkdir -p "$matrix_root/synapse"
mkdir -p "$matrix_root/registrations"
mkdir -p "$matrix_root/bridges/whatsapp"
mkdir -p "$matrix_root/bridges/telegram"
mkdir -p "$matrix_root/caddy/data"
mkdir -p "$matrix_root/caddy/config"

echo "matrix root: $matrix_root"
