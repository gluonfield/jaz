# Activepieces Local Stack

This directory runs Activepieces as the integration/action bus for Jaz.

Activepieces owns connectors, OAuth connections, triggers, and outbound actions.
Jaz/jazmem owns memory. Flows should drop normalized files into `io/` for a coding
agent to process into `~/.jaz/memory`.

## Start

```bash
cd /Users/wins/Projects/personal/jarvis/jaz/activepieces
docker compose up -d
```

Open:

```text
http://localhost:8090
```

Stop:

```bash
docker compose down
```

Check status:

```bash
docker compose ps
curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8090
```

Tail logs:

```bash
docker compose logs -f app
```

Recreate containers without losing integrations:

```bash
docker compose pull
docker compose up -d --remove-orphans
```

Do not delete `.env` or `data/postgres` unless you intentionally want to
reset Activepieces and lose configured integrations.

## UI Setup

Open:

```text
http://localhost:8090
```

On first run, create the owner/admin account in the Activepieces UI.

To add integrations:

1. Create or open a flow.
2. Add a trigger or action for the app, such as Gmail, Slack, Notion, or Google Drive.
3. In that step's connection/auth field, choose to create a new connection.
4. Complete the OAuth/API-key flow in the browser.
5. Save and test the step.

OAuth connections, flow definitions, project settings, and encrypted connection
secrets are stored in Postgres under `data/postgres`. The encryption key that
can decrypt those connection secrets is in `.env` as `AP_ENCRYPTION_KEY`, so
that file must survive container rebuilds.

For local development, `AP_FRONTEND_URL` is set to:

```text
http://localhost:8090
```

Webhook triggers from external services need a URL the external service can
reach. For localhost development, use a tunnel such as ngrok and set
`AP_FRONTEND_URL` in `.env` to the tunnel URL, then restart:

```bash
docker compose up -d
```

## Local Data Layout

```text
.env         local secrets; keep this to preserve/decrypt integrations
data/
  cache/       Activepieces runtime cache
  postgres/    Activepieces database: users, flows, connections, encrypted tokens
  redis/       Activepieces queue state
io/
  raw/                 raw connector archives
  imports/pending/     normalized files for the coding agent
  imports/done/        processed imports
  imports/failed/      imports the agent could not process
  outbox/pending/      action requests from the agent
  outbox/done/         completed action requests
  outbox/failed/       failed action requests
```

The Activepieces app container mounts `io/` at:

```text
/jaz-io
```

For v0, use Activepieces flows to write files under `/jaz-io/imports/pending`
or `/jaz-io/raw`. A Code step can write files because this local stack uses
`AP_EXECUTION_MODE=UNSANDBOXED`.

Container lifecycle:

- `docker compose down` removes containers/network only; local `.env`, `data/`,
  and `io/` remain.
- `docker compose up -d --remove-orphans` recreates containers and reuses the
  persisted database and queue data.
- Deleting `data/postgres` resets Activepieces state.
- Deleting `.env` can make existing encrypted connection secrets unusable.
- Deleting `io/` deletes Jaz import/outbox files, not Activepieces connections.

Cold backup:

```bash
docker compose stop
tar -czf "$HOME/jaz-activepieces-backup-$(date +%Y%m%d).tgz" .env data io
docker compose up -d
```

Cold restore:

```bash
docker compose down
tar -xzf "$HOME/jaz-activepieces-backup-YYYYMMDD.tgz"
docker compose up -d
```

## Suggested File Units

Use one pending file per semantic unit rather than one message per file:

```text
slack_thread
gmail_thread
notion_page
drive_document
linkedin_message_thread
x_thread
```

Raw archives can be JSONL. Agent work units should be one JSON file per unit:

```text
io/raw/slack/2026-06-08.jsonl
io/imports/pending/slack_thread_2026-06-08_T123_C123_1710000000.json
```

## Activepieces MCP

Activepieces MCP is URL/HTTP based. It is not a local stdio command that this
repo runs. The MCP client connects to the Activepieces server URL.

After creating the first Activepieces account, enable MCP in:

```text
Settings -> MCP Server
```

Toggle the MCP server on, choose the tool categories this project should expose,
then copy the server URL. Activepieces documents the MCP endpoint as:

```text
https://your-instance.com/mcp
```

For this local stack, expect:

```text
http://localhost:8090/mcp
```

MCP client config shape:

```json
{
  "mcpServers": {
    "activepieces": {
      "url": "http://localhost:8090/mcp"
    }
  }
}
```

Authentication is handled by the MCP client via OAuth. The client opens a
browser the first time it connects. Activepieces does not return app credentials,
OAuth refresh tokens, or API keys through MCP tools; app connections should be
created in the Activepieces UI.

The useful MCP tools are:

- `ap_research_pieces`: discover available pieces and their actions/triggers.
- `ap_get_piece_props`: get the exact input schema for one action/trigger.
- `ap_list_connections`: list connected OAuth/app accounts.
- `ap_run_action`: run one action once, such as sending a Slack message.
- `ap_build_flow`: create a persistent flow for scheduled/webhook automation.

For Jaz v0, prefer file outbox workflows for writes. Direct MCP actions are useful
later, after Jaz has an MCP client and write approvals are explicit.

## References

- Activepieces Docker Compose: https://www.activepieces.com/docs/install/options/docker-compose
- Activepieces environment variables: https://www.activepieces.com/docs/install/configuration/environment-variables
- Activepieces architecture: https://www.activepieces.com/docs/install/architecture
- Activepieces MCP overview: https://www.activepieces.com/docs/mcp/overview
- Activepieces MCP tools: https://www.activepieces.com/docs/mcp/tools
