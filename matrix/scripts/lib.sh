#!/usr/bin/env sh

user_home() {
	home="$(eval "printf %s ~$(id -un)")"
	if [ -d "$home" ]; then
		printf "%s" "$home"
	else
		printf "%s" "$HOME"
	fi
}

runtime_root() {
	printf "%s" "${JAZ_RUNTIME_ROOT:-$(user_home)/.jaz}"
}

matrix_root() {
	printf "%s" "${JAZ_MATRIX_ROOT:-$(runtime_root)/node/matrix}"
}

configured_server_name() {
	config="$(matrix_root)/synapse/homeserver.yaml"
	if [ -f "$config" ]; then
		sed -n 's/^server_name: "\(.*\)"$/\1/p' "$config" | head -n 1
	fi
}

compose() {
	if docker compose version >/dev/null 2>&1; then
		docker compose "$@"
	else
		docker-compose "$@"
	fi
}

random_hex() {
	od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
}

yaml_quote() {
	printf "%s" "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/^/"/; s/$/"/'
}
