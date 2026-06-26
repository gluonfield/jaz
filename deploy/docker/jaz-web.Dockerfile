# syntax=docker/dockerfile:1.7
#
# jaz-web: the static Jaz browser app for Ink (and any static host).
# Built WITHOUT VITE_JAZ_API_URL, so it does not assume a backend at its own
# origin. The app connects to whatever backend the URL supplies via the
# #server=<backend>&key=<key> fragment (the key never reaches this host), then
# remembers it in that origin's localStorage.
#
#   docker buildx build -f deploy/docker/jaz-web.Dockerfile \
#     -t augustinast/testing:jaz-web --push .
#
# Build context is the Jaz repo root (for frontend/ and deploy/docker/* files).

FROM oven/bun:1.3.5 AS web
WORKDIR /src/frontend
# electron is a dev dep of the desktop app; skip its big binary download.
ENV ELECTRON_SKIP_BINARY_DOWNLOAD=1
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build:web

# Serve the SPA. Ink terminates TLS at its edge, so this is plain HTTP.
FROM caddy:2-alpine AS runtime
COPY --from=web /src/frontend/dist-web /srv
COPY deploy/docker/jaz-web.Caddyfile /etc/caddy/Caddyfile
ENV PORT=8080
EXPOSE 8080
# The caddy image's default command already runs /etc/caddy/Caddyfile.
