# syntax=docker/dockerfile:1.7
#
# jaz-backend: the API-only Jaz backend for Ink (and any container host).
# It needs no source build — it installs the published release binary and adds
# Node/npm so the built-in ACP agents (Codex, Claude, OpenCode) can launch.
#
#   docker buildx build -f deploy/docker/jaz-backend.Dockerfile \
#     --build-arg JAZ_VERSION=v0.0.69 -t augustinast/testing:jaz-backend --push .
#
# Build context is the Jaz repo root (for deploy/docker/* files).

FROM node:22-bookworm-slim

# linux/amd64 -> amd64, linux/arm64 -> arm64 (set by buildx).
ARG TARGETARCH
# Release tag to install, or "latest".
ARG JAZ_VERSION=latest

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git tini \
 && rm -rf /var/lib/apt/lists/*

# Backend Unix user. Agents run as this user and see HOME=/var/lib/jaz.
RUN useradd --system --uid 10001 --home-dir /var/lib/jaz --shell /usr/sbin/nologin jaz \
 && install -d -o jaz  -g jaz  -m 755 /var/lib/jaz /var/lib/jaz/workspaces/default \
 && install -d -o root -g root -m 755 /opt/jaz/bin /etc/jaz

# Install the published backend binary for the target architecture, verifying
# its checksum (mirrors docs/remote-backend.md "Server Setup").
RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
      amd64|arm64) arch="${TARGETARCH:-amd64}";; \
      *) echo "unsupported architecture: ${TARGETARCH}"; exit 1;; \
    esac; \
    if [ "${JAZ_VERSION}" = "latest" ]; then \
      base="https://github.com/gluonfield/jaz/releases/latest/download"; \
    else \
      base="https://github.com/gluonfield/jaz/releases/download/${JAZ_VERSION}"; \
    fi; \
    asset="jaz-backend-linux-${arch}.tar.gz"; \
    cd /tmp; \
    curl -fsSLO "${base}/${asset}"; \
    curl -fsSLO "${base}/${asset}.sha256"; \
    test "$(awk '{print $1}' "${asset}.sha256")" = "$(sha256sum "${asset}" | awk '{print $1}')"; \
    tar -xzf "${asset}"; \
    install -o root -g root -m 755 jaz /opt/jaz/bin/jaz; \
    rm -f "${asset}" "${asset}.sha256" jaz

COPY deploy/docker/application.yaml          /etc/jaz/application.yaml
COPY deploy/docker/jaz-backend-entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENV APPLICATION_CONFIG=/etc/jaz/application.yaml \
    HOME=/var/lib/jaz \
    JAZ_LOG=info \
    PORT=5299
VOLUME ["/var/lib/jaz"]
EXPOSE 5299

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS "http://127.0.0.1:${PORT}/health" || exit 1

ENTRYPOINT ["/usr/bin/tini", "-g", "--", "/usr/local/bin/entrypoint.sh"]
