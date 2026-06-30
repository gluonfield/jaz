# syntax=docker/dockerfile:1.7
#
# jaz-fullstack-custom: single-origin Jaz with the Leeroo-inspired light theme.
#
#   docker buildx build -f deploy/docker/jaz-fullstack-custom.Dockerfile \
#     --build-arg JAZ_VERSION=v0.0.75 -t augustinast/testing:jaz-fullstack-custom --push .
#
# Build context is the Jaz repo root.

FROM oven/bun:1.3.5 AS web
WORKDIR /src/frontend
ENV ELECTRON_SKIP_BINARY_DOWNLOAD=1
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN <<'EOF'
cat > ./src/renderer/public/jaz-defaults.js <<'JS'
// Leeroo-flavoured Jaz defaults for the full-stack custom image.
// Loaded before first paint by frontend/src/renderer/index.html.
window.__JAZ_DEFAULTS__ = {
  theme: 'light',
  uiFont: 'Inter',
  monoFont: 'JetBrains Mono',
  effects: true,
  wideLayout: true,
  scheme: {
    light: {
      accent: '#347ee8',
      background: '#ffffff',
      foreground: '#344154',
      contrast: 42,
    },
    dark: {
      accent: '#6ca6ff',
      background: '#111827',
      foreground: '#eef4ff',
      contrast: 52,
    },
  },
}
JS
EOF
RUN VITE_JAZ_API_URL=origin bun run build:web

FROM caddy:2-alpine AS caddy

FROM node:22-bookworm-slim AS runtime

ARG TARGETARCH
ARG JAZ_VERSION=latest

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git tini \
 && rm -rf /var/lib/apt/lists/*

COPY --from=caddy /usr/bin/caddy /usr/bin/caddy

RUN useradd --system --uid 10001 --home-dir /var/lib/jaz --shell /usr/sbin/nologin jaz \
 && install -d -o jaz  -g jaz  -m 755 /var/lib/jaz /var/lib/jaz/workspaces/default \
 && install -d -o root -g root -m 755 /opt/jaz/bin /etc/jaz

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

COPY --from=web /src/frontend/dist-web /srv
COPY deploy/docker/application.yaml                  /etc/jaz/application.yaml
COPY deploy/docker/jaz-backend-entrypoint.sh         /usr/local/bin/jaz-backend-entrypoint.sh
COPY deploy/docker/jaz-fullstack-custom-entrypoint.sh /usr/local/bin/entrypoint.sh
COPY deploy/docker/jaz-fullstack-custom.Caddyfile    /etc/caddy/Caddyfile
RUN chmod +x /usr/local/bin/jaz-backend-entrypoint.sh /usr/local/bin/entrypoint.sh

ENV APPLICATION_CONFIG=/etc/jaz/application.yaml \
    HOME=/var/lib/jaz \
    JAZ_LOG=info \
    PORT=8080 \
    JAZ_BACKEND_PORT=5299
VOLUME ["/var/lib/jaz"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS "http://127.0.0.1:${PORT}/health" || exit 1

ENTRYPOINT ["/usr/bin/tini", "-g", "--", "/usr/local/bin/entrypoint.sh"]
