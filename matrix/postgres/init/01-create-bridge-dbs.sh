#!/usr/bin/env sh
set -eu

for db in mautrix_whatsapp mautrix_telegram; do
	psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
	SELECT 'CREATE DATABASE $db'
	WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$db')\gexec
	SQL
done
