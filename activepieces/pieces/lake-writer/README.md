# lake-writer (Activepieces custom piece)

One reusable flow action, **Store to Lake**: writes an incoming item as a
markdown file with provenance frontmatter into the local data lake (or the
jazmem inbox for explicit captures) and appends a journal line for incremental
agent triage. Idempotent — file names derive from the external id, so trigger
replays are no-ops.

This directory is the source of truth; the Activepieces monorepo checkout is a
build environment only.

## Why a custom piece

Activepieces has no native local-disk storage: the built-in Storage piece is a
key-value store in its Postgres, and the file pieces target remote stores
(S3/Drive/SFTP). A custom piece is the supported way to reuse one action across
flows without copy-pasting code steps.

## Build and install

Container setup (mounts, `AP_EXECUTION_MODE=UNSANDBOXED`, port 8081) lives in
`../../docker-compose.yml`. Installation is automatic: the compose
`piece-installer` one-shot uploads `dist/*.tgz` on every `docker compose up`.

```bash
./build.sh   # one-time, and after every src/ change (bump version first)
```

`build.sh` clones a shallow `activepieces` checkout to
`~/Projects/vendor/activepieces` (override with `AP_DIR`), scaffolds
`packages/pieces/custom/lake-writer` on first run (`npm run cli pieces create`
— answer: name `lake-writer`, type `custom`), overlays `src/` + `package.json`
from here, runs `npm run build-piece lake-writer`, and copies the `.tgz` into
`./dist/` where the installer looks.

Community edition note: the pieces admin UI is enterprise-only, but
`POST /api/v1/pieces` with `packageType=ARCHIVE` works in CE — that is what
the installer does. For hot-reload development instead: run the monorepo with
`AP_DEV_PIECES=lake-writer npm start`.

## Use in flows

Every flow becomes: **trigger → (filter) → Store to Lake**, mapping trigger
fields to: `source`, `externalId`, `occurredAt`, `title`, `body`, optional
`url`/`people`, and `destination` (lake by default; jazmem inbox only for
explicit "remember this" captures, e.g. the Chrome-extension webhook flow).

Output contract:

- Lake file: `/data/<source>/YYYY/MM/YYYY-MM-DD-<slug>-<hash8>.md`
- Inbox file: `/memory-inbox/YYYY-MM-DD-<slug>-<hash8>.md` (jazmem reindexes within a minute)
- Journal: `/data/.journal/<write-date>.jsonl`, one line per stored file —
  `{path, source, destination, occurred_at, title}` — consumed by the jaz
  triage loop via its cursor.
