# Remote Backend

Jaz can run with the backend on a server and desktop, browser, or mobile clients on user devices. The backend owns sessions, memory, tools, credentials, workspaces, and connected-device policy. Clients are control surfaces.

## Server Setup

Install a release backend on a Linux server:

```sh
ssh root@SERVER
RELEASE=v0.0.46

apt-get update
apt-get install -y ca-certificates curl tar nodejs npm

useradd --system --home-dir /var/lib/jaz --create-home --shell /usr/sbin/nologin jaz || true
install -d -o root -g root -m 755 /opt/jaz/bin /etc/jaz
install -d -o jaz -g jaz -m 755 /var/lib/jaz /var/lib/jaz/workspaces/default

tmp=$(mktemp -d)
cd "$tmp"
arch=$(dpkg --print-architecture)
case "$arch" in amd64|arm64) ;; *) echo "unsupported architecture: $arch"; exit 1;; esac
asset="jaz-backend-linux-${arch}.tar.gz"
base=https://github.com/gluonfield/jaz/releases/download/$RELEASE
curl -fsSLO "$base/$asset"
curl -fsSLO "$base/$asset.sha256"
test "$(awk '{print $1}' "$asset.sha256")" = "$(sha256sum "$asset" | awk '{print $1}')"
tar -xzf "$asset"
install -o root -g root -m 755 jaz /opt/jaz/bin/jaz
```

Node/npm are required when the backend runs default ACP agents because the
built-in Codex, Claude, and OpenCode adapters launch through `npx`. Install each
agent CLI you enable on the server too, for example `npm install -g @openai/codex`
for Codex login. Without the CLI on the server's `PATH` the onboarding agent
cards show "Not installed" and offer no sign-in.

Agent OAuth on a remote backend can't use the local-loopback capture a desktop
login relies on: the CLI runs on the server, so the browser's redirect never
reaches it. The CLI falls back to printing an authentication code for you to
paste back. The onboarding sign-in card surfaces a field for that code and
relays it to the login process over `POST /v1/acp/auth-logins/{id}/input`; or
sign in with an API key instead, which needs no browser round-trip.

Agents run as the backend Unix user. With the service below, shell tools see
`HOME=/var/lib/jaz`; they do not read `/home/your-login-user` dotfiles, SSH keys,
GitHub auth, npm/go caches, or CLI config. Put shell-tool credentials under
`/var/lib/jaz`, use Jaz onboarding for managed agent profiles such as
`/var/lib/jaz/acp/codex-home` and `/var/lib/jaz/acp/claude`, configure MCP
servers in Jaz Settings, or run the service as the Unix user whose home should be
exposed to agents.

Write `/etc/jaz/application.yaml`:

```yaml
jaz:
  root: /var/lib/jaz
  workspace: /var/lib/jaz/workspaces/default
  memory:
    scheduler: true
```

Write `/etc/jaz/jaz.env`:

```sh
APPLICATION_CONFIG=/etc/jaz/application.yaml
JAZ_LOG=info
HOME=/var/lib/jaz
JAZ_ADDR=:5299
JAZ_PUBLIC_URL=https://jaz.example.com
```

Add `/etc/systemd/system/jaz.service`:

```ini
[Unit]
Description=Jaz backend
Wants=network-online.target
After=network-online.target

[Service]
User=jaz
Group=jaz
WorkingDirectory=/var/lib/jaz
EnvironmentFile=/etc/jaz/jaz.env
ExecStart=/opt/jaz/bin/jaz --addr ${JAZ_ADDR} --public-url ${JAZ_PUBLIC_URL}
Restart=on-failure
RestartSec=2
KillSignal=SIGINT
TimeoutStopSec=20
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/var/lib/jaz

[Install]
WantedBy=multi-user.target
```

Start it:

```sh
systemctl daemon-reload
systemctl enable --now jaz
systemctl status jaz --no-pager
systemctl show jaz -p Restart -p UnitFileState --no-pager
```

`enable --now` starts Jaz immediately and on future boots. `Restart=on-failure`
restarts the backend after crashes, non-zero exits, and unexpected signals
without fighting an intentional `systemctl stop jaz`.

Check the installed backend version:

```sh
/opt/jaz/bin/jaz --version
```

Update a release-installed backend binary:

```sh
sudo /opt/jaz/bin/jaz update --latest
sudo systemctl restart jaz
```

Use `jaz update --version v0.0.46` to install a specific release. The update
command downloads the matching Linux/macOS backend archive from GitHub, verifies
its `.sha256`, and replaces only the current executable.

For a source build, compile on a Go 1.26 machine, copy the binary to the server,
and install it to `/opt/jaz/bin/jaz` before restarting the service:

```sh
cd backend
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o jaz ./cmd/jaz
scp jaz root@SERVER:/tmp/jaz
ssh root@SERVER 'install -o root -g root -m 755 /tmp/jaz /opt/jaz/bin/jaz && systemctl restart jaz'
```

If the backend should be reachable directly, open the port:

```sh
ufw allow 5299/tcp comment "Jaz backend"
```

Prefer putting Caddy, nginx, or a tunnel in front of the backend for TLS instead of exposing plain HTTP. Keep runtime state under `/var/lib/jaz`; do not depend on paths from any client machine being visible to the server or agents.

The backend is API-only. It does not serve the React web app or static assets.

## Configuration

Provider and voice secrets can come from environment variables, `/etc/jaz/jaz.env`, or the Jaz runtime `.env` file managed by Settings:

```sh
OPENROUTER_API_KEY=...
OPENAI_API_KEY=...
MISTRAL_API_KEY=...
```

## Clients

The Electron desktop app can connect to a remote backend with a client URL or a pinned development backend:

```sh
JAZ_API_URL=https://jaz.example.com bun run dev
```

The browser client is a static web app. Host `frontend/dist-web` on any static host, for example `https://web.jaz.chat` or a self-hosted origin. It connects directly to the backend URL the user supplies; the backend is not involved in serving the web app.

For first setup from a browser, prefer a fragment URL so the bootstrap key is not sent to the static web host in the HTTP request:

```text
https://web.jaz.chat/#server=https%3A%2F%2Fjaz.example.com&key=...
```

Query URLs still work, but they are best kept for trusted/self-hosted web clients:

```text
https://web.jaz.chat/?server=https%3A%2F%2Fjaz.example.com&key=...
```

After a successful connection, the browser stores the backend URL and device token in that browser origin's `localStorage`, keyed by backend URL. Refreshing the page reconnects to the same backend; switching from `web.jaz.chat` to a self-hosted copy starts with separate browser storage.

A production browser origin may call a private or loopback backend, for example `http://localhost:5299`, as long as the browser allows the request. Jaz answers normal CORS preflights and Chrome Private Network Access preflights with `Access-Control-Allow-Private-Network: true`.

### Self-host the app and backend behind one origin

You only need this to reach the backend from a **browser** client. The Electron desktop app connects straight to a remote backend over plain HTTP, so it needs none of this — TLS and a reverse proxy matter only because a browser blocks an HTTPS page from calling a plain-HTTP backend (mixed content).

To expose a single surface and keep the backend private, serve the web build and proxy the API from one origin. The browser only ever talks to that origin, so there is no mixed content and no CORS, and the backend never leaves loopback. Requests still originate in the browser; the proxy is pure routing, not a server that makes calls for it.

Build the web app to target whatever origin it is served from:

```sh
VITE_JAZ_API_URL=origin bun run build:web   # emits frontend/dist-web
```

`origin` makes the app use `window.location.origin` as its backend, so one build works at any domain. (Set `VITE_JAZ_API_URL` to an explicit URL to pin a backend instead, or leave it unset to default to a local backend.)

Bind the backend to loopback so it is reachable only through the proxy — set `JAZ_ADDR=127.0.0.1:5299` and `JAZ_PUBLIC_URL=https://jaz.example.com` in `/etc/jaz/jaz.env`, restart `jaz`, and leave `:5299` out of the firewall (open only the proxy's `:443`/`:80`).

Serve the static build and proxy the API with Caddy:

```caddy
jaz.example.com {
    @api path /v1/* /health
    handle @api {
        reverse_proxy 127.0.0.1:5299
    }
    handle {
        root * /var/www/jaz-web
        try_files {path} /index.html
        file_server
    }
}
```

Install Caddy, save the block above as `/etc/caddy/Caddyfile`, and start it. The package installs a systemd service, so after editing the config just reload it:

```sh
sudo apt-get install -y caddy   # or download from caddyserver.com/download
sudo systemctl reload caddy     # applies the config; Caddy auto-provisions the TLS cert
```

To run Caddy directly instead of via the service — foreground for testing, or `caddy start` to background it:

```sh
sudo caddy run --config /etc/caddy/Caddyfile
```

Caddy upgrades the websocket/SSE streams automatically. Open `https://jaz.example.com?key=...` (the key from `/var/lib/jaz/client-url.txt`); the app connects to its own origin, which the proxy forwards to the private backend.

Remote `--public-url` server logs print the public base URL and the auth file path, not the root key:

```text
client: https://jaz.example.com
client key: /var/lib/jaz/auth.json
```

Keep the backend first-setup URL in a root-owned file such as `/var/lib/jaz/client-url.txt`:

```text
https://jaz.example.com?key=...
```

Use that URL directly in the desktop app, or convert it to the browser fragment form above. The root key is a bootstrap/recovery credential. Normal clients should store their own per-device token after registration. Local development runs without `--public-url` may still print the full client URL so the desktop launcher can start and connect to a local backend.

## Connected Devices

Remote Jaz behaves like WhatsApp or Telegram: a new desktop or mobile client can request access, but it is not trusted until an already approved device approves it. Settings shows connected devices and pending requests.

Migration 20 stores device records instead of relying only on one shared API key:

```text
device
  id
  name
  kind              desktop | mobile | browser | cli
  status            pending | approved | revoked
  public_key
  platform
  device_family
  model_identifier
  token_hash
  created_at
  approved_at
  revoked_at
  last_seen_at
  last_seen_ip
  user_agent
  app_version
```

Long-lived client tokens are random bearer tokens, hashed at rest, scoped to one device, and revocable without rotating every other device. The root key is only for bootstrap/recovery, not the token stored by normal clients.

Migration 21 adds stable client identity. The desktop app generates an Ed25519 keypair, stores the private key locally, sends only the raw public key to the backend, and uses `sha256(public_key)` as `device_id`. The bearer token still authorizes normal HTTP requests; the device identity names the installation and leaves room for signed repair/recovery later.

Migration 22 adds display metadata to devices: platform, device family, and model identifier. These fields are not authentication material. They exist so Settings and `jaz devices` can show recognizable pending devices such as `Johny / macOS / Mac / MacBookPro18,3`.

## Pairing Flow

First owner device:

1. Backend starts with no approved devices.
2. The owner retrieves the full client URL from the server, for example `ssh root@SERVER 'cat /var/lib/jaz/client-url.txt'`.
3. The first desktop app connects with that URL, or the first browser client opens the static web app with `#server=<backend>&key=<key>`.
4. The backend exchanges the root key for a per-device token and creates the first approved device.
5. The desktop stores only the per-device token for normal use.

Additional devices:

1. New client enters the backend URL or scans a QR code.
2. Client creates a pairing request with its stable `device_id`, public key, generated request secret, display name, app kind, platform, device family, model identifier, and app version.
3. Backend stores the request as pending and returns a request ID.
4. Client polls the request with its request secret.
5. Settings on an approved device shows the pending request with enough context to recognize it.
6. Owner approves or rejects it.
7. Approval lets the pending client finish with the token scoped to that one device identity; rejection leaves the client unauthenticated.

QR flow:

- A QR shown by an approved client should carry a short-lived pairing approval or pairing session URL, not the root backend key.
- A future QR scanned from a server terminal can be the first-device setup flow, but it should be one-time and expire quickly.

SSE and websocket follow-up:

- Do not put long-lived device bearer tokens in stream URLs.
- Add a short-lived stream token endpoint for EventSource and websocket URLs, or use a transport that can set `Authorization` headers.
- Existing `?key=` support can stay as a compatibility path during migration.

## API Shape

Keep device auth behind its own feature boundary:

```text
backend/internal/deviceauth
backend/internal/httpapi/devices
backend/internal/storage
backend/internal/storage/sqlite/queries/devices
backend/internal/storage/sqlite/generated/devices
```

Existing endpoints:

```text
GET    /v1/devices
POST   /v1/devices/register
PATCH  /v1/devices/{id}
DELETE /v1/devices/{id}

POST   /v1/devices/pairing-requests
GET    /v1/devices/pairing-requests/{id}
POST   /v1/devices/pairing-requests/{id}/approve
POST   /v1/devices/pairing-requests/{id}/reject
```

Unauthenticated endpoints are narrow: health, pairing request creation, and pairing poll by request secret. The root key can register the first approved device only while no approved devices exist; once one exists, root-key registration creates a pending request. Everything else requires an approved device token.

The auth middleware should resolve a request into an actor like:

```text
auth.Context
  device_id
  device_name
  approved
```

Feature services should receive that actor explicitly where policy depends on it. They should not parse HTTP headers.

## Settings UI

Settings includes a Devices section:

- Approved devices: name from client metadata, OS/platform details, model, app version, current-device marker, last seen, IP, revoke.
- Pending devices: requested name, OS/platform details, model, approximate time, IP, approve, reject.
- Revoked devices: collapsed history or hidden by default.
- Link device: future QR code plus short pairing code.

General Settings can keep local appearance and launch behavior. Device management deserves its own settings section once pairing exists because it is security state, not a cosmetic preference.

The UI does not show the root backend key as the normal way to connect. The first client uses the server-side setup URL, then the app stores a per-device token.

## Device CLI

The backend binary can inspect and approve devices directly against the Jaz root:

```sh
jaz devices --root /var/lib/jaz
jaz devices --root /var/lib/jaz approve <pairing-or-device-id>
```

If `--root` is omitted, the command uses the configured `jaz.root`, so on a systemd host this also works with the service config environment:

```sh
APPLICATION_CONFIG=/etc/jaz/application.yaml jaz devices
APPLICATION_CONFIG=/etc/jaz/application.yaml jaz devices approve <pairing-or-device-id>
```

The listing includes display metadata for each approved device and pending request. Approval accepts either the pending pairing request ID or the pending device ID shown by `jaz devices`.

## Migration

1. Keep accepting the existing backend API key while no approved devices exist.
2. Add device records and per-device tokens.
3. When the first client connects with the root key, convert it into an approved device token.
4. Once at least one approved device exists, make new root-key connections create pending devices instead of silently becoming trusted.
5. Eventually reserve the root key for local recovery and explicit setup only.
