# activepieces (jarvis connection hub)

Self-hosted Activepieces serving two surfaces:

- **Pull**: scheduled flows → `Store to Lake` piece → `~/data/<source>/...` markdown
  files + `~/data/.journal/*.jsonl` (the jaz triage loop's incremental feed).
- **Act**: AP's MCP server exposes write actions (gmail send, slack post,
  linear create) to jaz via its MCP runtime.

## First run

```bash
cp .env.example .env        # fill in (openssl rand -hex 16 / -hex 32)
./pieces/lake-writer/build.sh   # one-time: builds the custom piece .tgz
docker compose up -d
```

That's it — the `piece-installer` one-shot container waits for AP, creates the
admin account from `AP_EMAIL`/`AP_PASSWORD` on first boot (signs in thereafter),
and uploads `pieces/lake-writer/dist/*.tgz` if that version isn't installed.
It re-runs on every `up` and exits immediately when nothing to do.

UI: http://localhost:8081 (8080 belongs to jaz). Sign in with the `.env` creds.

## Updating the piece

1. Edit `pieces/lake-writer/src/`
2. Bump `version` in `pieces/lake-writer/package.json`
3. `./pieces/lake-writer/build.sh && docker compose up -d piece-installer`

## Layout

- `docker-compose.yml` — AP + postgres + redis + one-shot `piece-installer`.
  Mounts `~/data` → `/data` and `~/.jaz/memory/inbox` → `/memory-inbox`;
  `AP_EXECUTION_MODE=UNSANDBOXED` so the piece can write files.
- `pieces/lake-writer/` — the custom piece (source of truth + build.sh + dist/).
  See its README for the action's contract and flow usage.
- `installer/install-piece.sh` — idempotent .tgz upload via `POST /api/v1/pieces`
  (works on community edition; the pieces admin UI is enterprise-only).

## Flows

Every pull flow: **trigger → (filter) → Store to Lake**. Start with Gmail
(New Email), Slack (New Message), Linear (Issue events), Calendar (daily
schedule → List Events → loop). Triggers only see items from enable-time
forward; backfill is a separate one-time concern.
