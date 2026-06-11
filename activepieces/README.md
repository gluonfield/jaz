# activepieces (jaz connection hub)

Self-hosted Activepieces serving two surfaces for jaz:

- **Pull**: scheduled flows → **Store to Ingest** piece → markdown files in
  `~/.jaz/ingest/incoming/`.
- **Act**: AP exposes write actions (gmail send, slack post, linear create) to jaz
  as MCP tools.

Stack: AP `${ACTIVEPIECES_VERSION}` on **http://localhost:8090** (jaz owns 8080),
pgvector postgres + redis, with AP runtime state under this directory
(`data/postgres`, `data/redis`, `data/cache`) and connector exports under
`${JAZ_INGEST_ROOT:-$HOME/.jaz/ingest}`.

## ⚠️ One-time after the directory move

The running containers were started from the old path
(`~/Projects/personal/jarvis/jaz/activepieces`) and their bind mounts still
point there. Recreate from
here (postgres data moved with the folder, so nothing is lost):

```bash
# 1. fill the two keys at the bottom of .env with your AP login
#    (the account you created in the UI): AP_ADMIN_EMAIL / AP_ADMIN_PASSWORD
# 2. set JAZ_LAKE_WRITER_ARCHIVE_URL, or bundle the .tgz under piece-archives/
# 3. recreate the stack from this directory:
docker-compose up -d
# 4. verify mounts now point here:
docker inspect jaz-activepieces-app --format '{{range .Mounts}}{{.Source}} -> {{.Destination}}{{"\n"}}{{end}}'
# 5. confirm the piece installed:
docker-compose logs piece-installer
```

After this, `docker-compose up -d` is the only command you ever need — the
`piece-installer` one-shot re-checks the piece on every `up` and exits
instantly when it's already installed.

## Directory map

| Path | What |
|---|---|
| `$JAZ_INGEST_ROOT/incoming/<source>/YYYY/MM/*.md` | connector queue — raw files not processed yet |
| `$JAZ_INGEST_ROOT/processed/<source>/YYYY/MM/*.md` | raw files already inspected by Jaz |
| `$JAZ_INGEST_ROOT/failed/<source>/YYYY/MM/*.md` | raw files that need attention after triage failed |
| `piece-archives/` | optional bundled Activepieces piece archives (`*.tgz`) |
| `installer/install-piece.sh` | idempotent piece upload (CE-compatible API call) |
| `data/` | AP runtime state: postgres, redis, piece cache |

Container mounts: `${JAZ_INGEST_ROOT:-$HOME/.jaz/ingest}` → `/jaz-ingest`,
`${JAZ_MEMORY_ROOT:-$HOME/.jaz/memory}/inbox` → `/memory-inbox`.
`AP_EXECUTION_MODE=UNSANDBOXED` (in `.env`) is required so the piece can write files.
`AP_INTERNAL_URL=http://127.0.0.1:80` lets app-created worker jobs call the
container-local API while `AP_FRONTEND_URL=http://localhost:8090` remains the
browser/OAuth URL.
Packaged installers can override `JAZ_INGEST_ROOT` and `JAZ_MEMORY_ROOT`, but
they must point at machine-local host paths, not an agent sandbox home.

## Adding a connection

UI (http://localhost:8090) → flow editor → add a piece step → **Connect**.

- **API-key services** (Linear, GitHub, ...): paste the key, done.
- **OAuth services** (Google, Slack): self-hosted AP needs *your own* OAuth app —
  one-time ~15 min per provider:
  - **Google**: GCP console → create project → enable Gmail/Calendar APIs →
    OAuth client (Web application), redirect URI `http://localhost:8090/redirect`
    → paste client id/secret into AP's connection dialog. Keep the consent
    screen in *testing* mode with yourself as test user — no verification needed.
  - **Slack**: api.slack.com → create app → add the scopes AP's connect dialog
    lists → same redirect URI pattern.

## Pull flows (dumping data)

Every pull flow is: **trigger → (optional filter) → Store to Ingest**. The piece
is idempotent (file name derives from `externalId`), so replays and overlapping
polls are safe.

Field mapping, Gmail example (trigger: *New Email*):

| Store to Ingest input | map from trigger |
|---|---|
| `source` | literal `gmail` |
| `externalId` | message id |
| `occurredAt` | message date (ISO) |
| `title` | subject |
| `body` | plain-text body |
| `url` | thread link (optional) |
| `people` | from/to addresses (optional) |
| `destination` | leave default (Jaz ingest incoming) |

Repeat the pattern: Slack *New Message* (`source=slack`, message ts as
externalId), Linear *Issue events* (`source=linear`, issue id), Calendar
(daily *Schedule* trigger → *List Events* → loop → Store to Ingest,
`source=calendar`, event id).

Notes: triggers only see items from enable-time forward (backfill is a
separate one-time concern); `AP_TRIGGER_DEFAULT_POLL_INTERVAL=5` minutes is
the pull latency. Items land on the host at
`$JAZ_INGEST_ROOT/incoming/...` within one poll. The folder is the queue:
triage moves retained raw files to `processed/` after inspection, or `failed/`
when processing needs attention.

The `destination` dropdown's second option (`/memory-inbox`) writes straight
into jazmem's inbox — only for explicit "remember this" capture flows (e.g. a
Chrome-extension webhook flow), never bulk pulls.

## Acting (agent writes)

Connections made for pulls are reusable for writes. Expose only the handful of
actions you need (gmail send, slack post, linear create) via AP's MCP server
(Settings → MCP in the UI) and register that URL in jaz (`/v1/mcp/servers`).
Draft-don't-send policy lives on the jaz side.

## Jaz ingest writer

The Activepieces piece source does not live in this repo and Jaz does not fork
Activepieces to ship it. The source/release owner is:

`https://github.com/gluonfield/jaz-activepieces-lake-writer`

That repository builds `activepieces-piece-lake-writer-<version>.tgz` on
release. Jaz installs the archive with Activepieces' `POST /api/v1/pieces` API.
Set these in `.env` for the target release:

```bash
JAZ_LAKE_WRITER_PIECE_NAME=@activepieces/piece-lake-writer
JAZ_LAKE_WRITER_PIECE_VERSION=0.1.2
JAZ_LAKE_WRITER_ARCHIVE_URL=https://github.com/gluonfield/jaz-activepieces-lake-writer/releases/download/v0.1.2/activepieces-piece-lake-writer-0.1.2.tgz
```

For offline/package installs, put the `.tgz` in `activepieces/piece-archives/`
instead of setting `JAZ_LAKE_WRITER_ARCHIVE_URL`.

## Troubleshooting

- Piece missing in the editor → `docker compose logs piece-installer`
  (no `.tgz` built? empty admin creds? AP not up yet — it waits ~4 min, then
  gives up; rerun with `docker-compose up -d piece-installer`).
- Flow fails writing files → re-check mounts (step 4 above) and that
  `AP_EXECUTION_MODE=UNSANDBOXED` survived any `.env` edit.
- Generate Sample Data hangs or fails with `127.0.0.1:8090` in logs →
  `AP_INTERNAL_URL` is missing or the app container needs recreating.
- Nothing in `$JAZ_INGEST_ROOT/incoming/` → flow ran before the recreate
  (old mount) or the trigger hasn't fired — check the flow's run history in the UI.
