# Remote Backend

Jaz can run with the backend on a server and desktop, browser, or mobile clients on user devices. The backend owns sessions, memory, tools, credentials, workspaces, and connected-device policy. Clients are control surfaces.

## Server Setup

Install a release backend on a Linux server:

```sh
ssh root@SERVER

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
base=https://github.com/gluonfield/jaz/releases/latest/download
curl -fsSLO "$base/$asset"
curl -fsSLO "$base/$asset.sha256"
test "$(awk '{print $1}' "$asset.sha256")" = "$(sha256sum "$asset" | awk '{print $1}')"
tar -xzf "$asset"
install -o root -g root -m 755 jaz /opt/jaz/bin/jaz
```

`releases/latest/download` always resolves to the newest release, so this recipe
stays current without editing. To pin a specific release instead, swap in
`releases/download/<tag>`, for example
`base=https://github.com/gluonfield/jaz/releases/download/v0.0.46`.

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
JAZ_ADDR=127.0.0.1:5299
JAZ_PUBLIC_URL=https://jaz.example.com
JAZ_PREVIEW_URL_TEMPLATE=https://{id}.preview.example.com
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
ExecStart=/opt/jaz/bin/jaz --addr ${JAZ_ADDR} --public-url ${JAZ_PUBLIC_URL} --preview-url-template ${JAZ_PREVIEW_URL_TEMPLATE}
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

Use `jaz update --version <tag>` to install a specific release. The update
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

Keep port 5299 private. Put Caddy, nginx or a tunnel in front of the loopback
listener and use HTTPS for remote clients. Keep runtime state under `/var/lib/jaz`;
client-local paths are not visible to the server or agents.

The backend is API-only. It does not serve the React web app or static assets.

### Remote web previews

Agent-started web servers bind to the backend machine, so remote clients must never open their own `localhost`. Jaz registers loopback-only targets through the authenticated `POST /v1/preview/proxies` endpoint and returns a random, eight-hour capability URL on an isolated origin. The capability host forwards ordinary HTTP methods and WebSocket upgrades used by development servers such as Vite. Jaz strips Authorization, cookies, device headers, Origin, and Referer before the request reaches the preview process.

Set `--preview-url-template` to an origin containing `{id}` exactly once in its hostname, for example:

```text
https://{id}.preview.example.com
```

The template is explicit: Jaz never derives it from `--public-url`. It must use HTTP or HTTPS and cannot contain credentials, a path, query, or fragment. A remote backend without this setting rejects preview registration with an actionable error. Local Electron development keeps working without it through isolated `jaz-preview-<id>.localhost` origins.

The deployment must provide all of the following for the template:

- Wildcard DNS for `*.preview.example.com` pointing to the same reverse proxy or tunnel as the Jaz backend.
- A TLS certificate covering `*.preview.example.com` when the template uses HTTPS.
- A wildcard virtual-host or tunnel ingress rule that forwards the hostname unchanged to the Jaz backend.
- HTTP upgrade forwarding so preview WebSockets and Vite HMR reach Jaz's reverse proxy.

For a locally managed Cloudflare Tunnel, the matching ingress shape is:

```yaml
ingress:
  - hostname: jaz.example.com
    service: http://localhost:5299
  - hostname: "*.preview.example.com"
    service: http://localhost:5299
  - service: http_status:404
```

Create a proxied wildcard DNS record for `*.preview.example.com` targeting the tunnel, and ensure Cloudflare edge certificate coverage includes that exact wildcard. Then validate routing before restarting `cloudflared`:

```sh
cloudflared tunnel ingress validate
cloudflared tunnel ingress rule https://0123456789abcdef0123456789abcdef.preview.example.com/.well-known/jaz-preview
dig +short 0123456789abcdef0123456789abcdef.preview.example.com
curl -i https://0123456789abcdef0123456789abcdef.preview.example.com/.well-known/jaz-preview
```

The rule command must select the wildcard Jaz service, DNS must resolve, and HTTPS must complete without a certificate error. An unregistered hostname may be rejected by Jaz authentication, but it must not return the tunnel's catch-all response. Browser and native mobile clients probe the registered capability origin before rendering, so incomplete DNS, TLS, or ingress configuration is shown as a preview error rather than a blank frame.

## Configuration

Provider and voice secrets can come from environment variables, `/etc/jaz/jaz.env`, or the Jaz runtime `.env` file managed by Settings:

```sh
OPENROUTER_API_KEY=...
OPENAI_API_KEY=...
MISTRAL_API_KEY=...
```

MCP servers can be declared so a deployment starts with them connected. `JAZ_MCP_SERVERS` is a JSON array in the shape the MCP server API accepts; each start adds new names, and existing ones get their declared settings back. Keep secrets in their own variables, such as the key a Jaz Tasks deployment provisions with `OWNER_API_KEY`:

```sh
JAZ_TASKS_API_KEY=...
JAZ_MCP_SERVERS='[{"name":"Tasks","url":"http://tasks:7400/mcp","bearer_token_env_var":"JAZ_TASKS_API_KEY"}]'
```

A server also takes `headers`, sent on every request: each has a `name` and a `value` or an `envvar`. The same JSON works as the string `jaz.mcpservers` in `application.yaml`; a YAML list is rejected. Jaz Tasks reads its [app configuration headers](https://github.com/gluonfield/jaz-tasks#configure-the-app) this way:

```yaml
jaz:
  mcpservers: |
    [{"name": "Tasks", "url": "http://tasks:7400/mcp", "bearer_token_env_var": "JAZ_TASKS_API_KEY",
      "headers": [{"name": "Tasks-View", "value": "board"}, {"name": "Tasks-Mode", "value": "minimal"}]}]
```

## Clients

The Electron desktop app can connect to a remote backend with a client URL or a pinned development backend:

```sh
JAZ_API_URL=https://jaz.example.com bun run dev
```

The browser client is a static web app. Host `frontend/dist-web` on any static host, for example `https://web.jaz.chat` or a self-hosted origin. It connects directly to the backend URL the user supplies; the backend is not involved in serving the web app.

An approved device can share a one-use enrollment link. To open it in the browser client, put its invitation in the fragment:

```text
https://web.jaz.chat/#server=https%3A%2F%2Fjaz.example.com&key=...
```


After a successful connection, the browser stores the backend URL and device token in that browser origin's `localStorage`, keyed by backend URL. Refreshing the page reconnects to the same backend; switching from `web.jaz.chat` to a self-hosted copy starts with separate browser storage.

Trusted browser origins may call a private or loopback backend, for example `http://localhost:5299`, as long as the browser allows the request. Jaz answers normal CORS preflights and Chrome Private Network Access preflights with `Access-Control-Allow-Private-Network: true`.

Additional self-hosted browser origins must be explicitly listed with `--allowed-origins=https://app.example.com`. The public backend origin, Jaz web origins and loopback development origins are supported by default.

### Self-host the app and backend behind one origin (optional)

You only need this to reach the backend from a **browser** client. The Electron desktop app connects straight to a remote backend over plain HTTP, so it needs none of this — TLS and a reverse proxy matter only because a browser blocks an HTTPS page from calling a plain-HTTP backend (mixed content).

One HTTPS origin serves the web build at `/` and reverse-proxies the API to a backend that stays on `localhost`. The browser only talks to that origin (no mixed content, no CORS) and the backend is never exposed; the proxy is pure routing — requests still originate in the browser. This assumes the backend is already installed and running (see [Server Setup](#server-setup)).

**1. Pick the public hostname Caddy will serve.** It must resolve to the server, because Caddy auto-provisions a Let's Encrypt cert for it. With a domain, point an A record at the server IP. With only an IP and no DNS, use [sslip.io](https://sslip.io) — it resolves `<ip>.sslip.io` to `<ip>` for free:

```sh
HOST=34.9.55.98.sslip.io        # or your own domain: jaz.example.com
```

**2. Build the web app on the server.** Clone into `/opt/jaz` and build with `VITE_JAZ_API_URL=origin`, which makes the app use `window.location.origin` as its backend (so it targets whatever host serves it). Caddy serves the build in place — no copy step. Fits comfortably on a 2 GB VM:

```sh
sudo apt-get install -y git unzip
curl -fsSL https://bun.sh/install | bash && export PATH="$HOME/.bun/bin:$PATH"
sudo git clone --depth 1 https://github.com/gluonfield/jaz.git /opt/jaz
sudo chown -R "$USER" /opt/jaz
cd /opt/jaz/frontend && bun install
POSTHOG_PROJECT_TOKEN=<ph_project_token> \
  VITE_JAZ_API_URL=origin \
  bun run build:web                                         # builds /opt/jaz/frontend/dist-web
```

**3. Point the backend at loopback** so it stays private and Caddy is the only thing exposed:

```sh
sudo sed -i "s#^JAZ_ADDR=.*#JAZ_ADDR=127.0.0.1:5299#; s#^JAZ_PUBLIC_URL=.*#JAZ_PUBLIC_URL=https://$HOST#" /etc/jaz/jaz.env
sudo systemctl restart jaz
```

Keep `:5299` out of the firewall; open only `:80` and `:443`.

**4. Install Caddy and write the proxy config** (on the server; the heredoc substitutes `$HOST` and leaves Caddy's `{...}` placeholders alone):

```sh
sudo apt-get install -y caddy        # or download from caddyserver.com/download
sudo tee /etc/caddy/Caddyfile >/dev/null <<EOF
$HOST {
    @api path /v1/* /health
    handle @api {
        reverse_proxy 127.0.0.1:5299
    }
    handle {
        root * /opt/jaz/frontend/dist-web
        try_files {path} /index.html    # SPA fallback — serve index.html for client routes (/new, /sessions/…)
        file_server
    }
}
EOF
sudo systemctl reload caddy          # or run directly: sudo caddy run --config /etc/caddy/Caddyfile
```

Caddy provisions the cert on first start (needs `:80`/`:443` reachable) and upgrades websocket/SSE streams automatically.

Two lines in that `handle` block are load-bearing. **`try_files {path} /index.html` is the single-page-app fallback:** routes like `/new` or `/sessions/<id>` are client-side only — no such file exists on disk — so opening one directly or hard-refreshing it would 404 without this. `try_files` serves the real asset when the path matches a file and otherwise rewrites the request to `index.html`, letting the app's router resolve the route in the browser. It must come **before** `file_server`. And the `@api` block must stay **above** the web `handle` so `/v1/*` and `/health` reach the backend instead of falling through to `index.html`. Verify with `curl -sI https://$HOST/new | head -1` — it should report `200` and `Content-Type: text/html`, not `404`. The same fallback is required on any static host that serves the web build (nginx: `try_files $uri /index.html;`). Hosts that read a `_redirects` file — Cloudflare Pages, Netlify — are already covered: the build ships one at the deploy root (`frontend/src/renderer/public/_redirects` → `dist-web/_redirects`) with `/*  /index.html  200`.

**5. Verify and connect.**

```sh
curl -fsS https://$HOST/health        # -> {"ok":true,...}
```

Open the server URL in Jaz and request device approval. For the first device on
a headless server, approve its pending request over SSH with `jaz devices approve`.
The desktop app can bootstrap locally from its protected root credential.

Server logs print the public base URL and protected auth file path. The root key
stays on the server for local bootstrap, recovery and authenticated agent MCP
connections; remote clients receive device credentials or bounded invitations.

## Connected Devices

Each installation has a random, revocable bearer credential, stored as a hash
on the server. Devices in Settings lists approved, pending and revoked devices.
Revoking a device closes its active streams and cancels its unused invitations.

**Add Device** produces a one-use enrollment invitation that expires in ten
minutes. The invitation sits in the link fragment and is exchanged for a separate
device credential. It carries no permanent root authority. Entering the base URL
instead creates a pending request that an approved device or server operator
must approve.

Pairing proof is sent in `X-Jaz-Pairing-Secret` and redeemed once for a fresh
credential. Browser event streams and embedded files use one-minute resource tickets
from the authenticated `/v1/auth/check` response. Native clients use Authorization
headers; browser WebSockets and the Chrome bridge use a WebSocket subprotocol. Durable `?key=` resource
authentication is rejected.

OAuth credentials are encrypted with AES-256-GCM and bound to their account ID.
The separate 32-byte key defaults to `<root>/secrets.key`; `JAZ_SECRET_KEY_FILE`
can point to a protected external key file. Preserve that key with secured
backups: a database copy alone cannot restore connected accounts. Existing
plaintext OAuth rows are encrypted on startup and SQLite plaintext pages are
purged. The runtime directory is private and DB/WAL files use mode 0600.
Desktop credentials use the OS encryption service; iOS uses Keychain.

Upgrade the server and desktop client together, stopping the old server before
opening its database with the upgraded binary. Existing per-device credentials
continue to work. Unused invitations and resource tickets expire on restart.

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
