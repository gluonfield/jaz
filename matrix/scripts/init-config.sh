#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."
. ./scripts/lib.sh

matrix_root="$(matrix_root)"
server_name="${SYNAPSE_SERVER_NAME:-}"
if [ -z "$server_name" ]; then
	server_name="$(configured_server_name || true)"
fi
if [ -z "$server_name" ] && [ "${JAZ_MATRIX_LOCAL:-}" = "1" ]; then
	server_name="localhost"
fi
if [ -z "$server_name" ]; then
	echo "SYNAPSE_SERVER_NAME is required on first init; set JAZ_MATRIX_LOCAL=1 only for localhost development." >&2
	exit 1
fi
postgres_user="${POSTGRES_USER:-jaz}"
postgres_db="${SYNAPSE_DB:-synapse}"

./scripts/init-dirs.sh >/dev/null

secret() {
	path="$matrix_root/secrets/$1"
	if [ ! -f "$path" ]; then
		random_hex > "$path"
		chmod 600 "$path"
	fi
	cat "$path"
}

postgres_password="$(secret postgres_password)"
registration_secret="$(secret registration_shared_secret)"
macaroon_secret="$(secret macaroon_secret_key)"
form_secret="$(secret form_secret)"

if [ ! -f "$matrix_root/synapse/$server_name.signing.key" ]; then
	export JAZ_MATRIX_ROOT="$matrix_root"
	SYNAPSE_SERVER_NAME="$server_name" compose -f docker-compose.yaml --profile setup run --rm synapse-generate >/dev/null
fi

appservices=""
for file in "$matrix_root"/registrations/*.yaml "$matrix_root"/registrations/*.yml; do
	if [ -f "$file" ]; then
		name="$(basename "$file")"
		appservices="${appservices}  - \"/data/appservices/$name\"
"
	fi
done

{
	printf "server_name: %s\n" "$(yaml_quote "$server_name")"
	cat <<EOF
pid_file: /data/homeserver.pid
web_client_location: null
public_baseurl: null
listeners:
  - port: 8008
    tls: false
    type: http
    x_forwarded: true
    resources:
      - names: [client]
        compress: false
database:
  name: psycopg2
  args:
    user: "$postgres_user"
    password: "$postgres_password"
    database: "$postgres_db"
    host: postgres
    cp_min: 5
    cp_max: 10
log_config: "/data/$server_name.log.config"
media_store_path: /data/media_store
uploads_path: /data/uploads
registration_shared_secret: "$registration_secret"
enable_registration: false
report_stats: false
macaroon_secret_key: "$macaroon_secret"
form_secret: "$form_secret"
signing_key_path: "/data/$server_name.signing.key"
trusted_key_servers:
  - server_name: "matrix.org"
EOF
	if [ -n "$appservices" ]; then
		printf "app_service_config_files:\n%s" "$appservices"
	fi
} > "$matrix_root/synapse/homeserver.yaml"

chmod 600 "$matrix_root/synapse/homeserver.yaml"
echo "matrix config: $matrix_root/synapse/homeserver.yaml"
