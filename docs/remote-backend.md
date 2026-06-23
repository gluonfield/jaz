# Remote Backend

Jaz can run with the backend on a server and desktop or mobile clients on user devices. The backend owns sessions, memory, tools, credentials, workspaces, and connected-device policy. Clients are control surfaces.

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
for Codex login.

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

## Configuration

Provider and voice secrets can come from environment variables, `/etc/jaz/jaz.env`, or the Jaz runtime `.env` file managed by Settings:

```sh
OPENROUTER_API_KEY=...
OPENAI_API_KEY=...
MISTRAL_API_KEY=...
```

The desktop app can connect to a remote backend with a client URL:

```sh
JAZ_API_URL=https://jaz.example.com bun run dev
```

Remote `--public-url` server logs print the public base URL and the auth file path, not the root key:

```text
client: https://jaz.example.com
client key: /var/lib/jaz/auth.json
```

Keep the full first-setup client URL in a root-owned file such as `/var/lib/jaz/client-url.txt`:

```text
https://jaz.example.com?key=...
```

The root key is a bootstrap/recovery credential. Normal clients should store their own per-device token after registration. Local development runs without `--public-url` may still print the full client URL so the desktop launcher can start and connect to a local backend.

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
3. The first desktop app connects with that URL.
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
