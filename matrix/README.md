# Matrix node

Server-side Matrix stack for Jaz chat/social integrations.

This is a sidecar stack for the always-on Jaz server host. The Electron client
does not run Docker or mount local files; it talks to the Jaz server, and the
server manages this stack on the same VM or laptop where it runs.

## Runtime paths

By default the stack stores state outside the repository:

| Path | What |
|---|---|
| `~/.jaz/node/matrix/postgres` | Postgres data |
| `~/.jaz/node/matrix/secrets` | generated local secrets |
| `~/.jaz/node/matrix/synapse` | Synapse config, media, signing keys |
| `~/.jaz/node/matrix/registrations` | Matrix appservice registration files |
| `~/.jaz/node/matrix/bridges/<bridge>` | bridge config and session state |

Jaz ingest data remains under the normal Jaz ingest root, such as
`~/.jaz/ingest/raw`. The Matrix sidecar stack does not write ingest records
directly.

Set `JAZ_RUNTIME_ROOT` or `JAZ_MATRIX_ROOT` to override these paths.

## Commands

On a VM, initialize with the Matrix server name that clients will use:

```bash
cd matrix
SYNAPSE_SERVER_NAME=matrix.example.com ./scripts/init-config.sh
./scripts/compose.sh config
```

For local development only:

```bash
cd matrix
JAZ_MATRIX_LOCAL=1 ./scripts/init-config.sh
JAZ_MATRIX_LOCAL=1 ./scripts/compose.sh config
```

Start the base services:

```bash
./scripts/compose.sh up -d postgres synapse
```

Bridge config generation and appservice registration are intentionally separate
from this scaffold. Jaz should generate bridge configs, write registration files
under `$JAZ_MATRIX_ROOT/registrations`, rerun:

```bash
./scripts/init-config.sh
```

then restart Synapse and start bridges with:

```bash
./scripts/compose.sh restart synapse
./scripts/compose.sh --profile bridges up -d
```

The public proxy is opt-in:

```bash
MATRIX_PUBLIC_HOST=matrix.example.com ./scripts/compose.sh --profile public up -d caddy
```

The public proxy only forwards Matrix client and media routes; Synapse admin
routes remain private to the Docker network and localhost bind.
