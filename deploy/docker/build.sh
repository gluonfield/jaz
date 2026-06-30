#!/usr/bin/env bash
# Build and push the Jaz Ink images from the Jaz repo root.
#
#   docker login -u augustinast          # once
#   REPO=augustinast/testing JAZ_VERSION=v0.0.69 deploy/docker/build.sh
#
# Override PLATFORM (default linux/amd64) for arm64 hosts.
set -euo pipefail

REPO="${REPO:-augustinast/testing}"
JAZ_VERSION="${JAZ_VERSION:-latest}"
PLATFORM="${PLATFORM:-linux/amd64}"

root="$(cd "$(dirname "$0")/../.." && pwd)"   # jaz repo root
cd "$root"

echo ">> building ${REPO}:jaz-backend (jaz ${JAZ_VERSION}, ${PLATFORM})"
docker buildx build --platform "${PLATFORM}" \
	-f deploy/docker/jaz-backend.Dockerfile \
	--build-arg JAZ_VERSION="${JAZ_VERSION}" \
	-t "${REPO}:jaz-backend" --push .

echo ">> building ${REPO}:jaz-web (${PLATFORM})"
docker buildx build --platform "${PLATFORM}" \
	-f deploy/docker/jaz-web.Dockerfile \
	-t "${REPO}:jaz-web" --push .

echo ">> building ${REPO}:jaz-fullstack-custom (${PLATFORM})"
docker buildx build --platform "${PLATFORM}" \
	-f deploy/docker/jaz-fullstack-custom.Dockerfile \
	--build-arg JAZ_VERSION="${JAZ_VERSION}" \
	-t "${REPO}:jaz-fullstack-custom" --push .

echo ">> done: ${REPO}:jaz-backend  ${REPO}:jaz-web  ${REPO}:jaz-fullstack-custom"
