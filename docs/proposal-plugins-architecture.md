# Unified Plugins Architecture

Unify Jaz’s **Connections** and **Plugins** into a single **Plugins** architecture and UI modelled after Claude Customize, Codex Plugins, and Antigravity Customizations. A **Plugin** is one portable bundle that can provide any combination of **connections (MCP or hardcoded Go integrations)**, **skills**, and **app views**, shared across local Jaz, Jaz Cloud, and Jaz Bots. **Settings → MCP servers** stays as its own dedicated section for user-added custom MCP servers.

## Contents

- [Core principles](#core-principles)
- [Unified Plugin model](#unified-plugin-model)
- [Multi-ecosystem format compatibility](#multi-ecosystem-format-compatibility)
- [Sources and marketplaces](#sources-and-marketplaces)
- [Runtime delivery (Local, Cloud and Bots)](#runtime-delivery-local-cloud-and-bots)
- [Unified Plugins UI](#unified-plugins-ui)
- [Backend refactor map](#backend-refactor-map)
- [Implementation plan and open decisions](#implementation-plan-and-open-decisions)

## Core principles

- **One Plugins hub (Connections + Plugins merged):** Collapse Settings **Connections** and **Plugins** into one **Plugins** catalog and UI. Whether a capability is WhatsApp/Gmail (hardcoded Go integration), Granola/CRM (official remote OAuth MCP), Base44 (skills from GitHub), or Tenders (skills + HTML app views), users discover, enable, connect, and inspect it in one place.
- **Keep MCP servers separate:** **Settings → MCP servers** remains its own Settings section (`/v1/mcp/servers`, `MCPSettings.tsx`) for user-added custom remote HTTP and local stdio MCP servers, untouched.
- **Zero Jaz-only wrapper boilerplate:** Drop `extensions["chat.jaz"]` completely. Standard fields (`name`, `displayName`, `description`, `version`, `logo`, `category`, `suggestedPrompts`/`defaultPrompt`, `author`, `homepage`, `repository`, `skills`, MCP servers) come directly from standard Codex, Claude, Antigravity, and Cursor manifests. A plugin that ships Jaz HTML views simply adds optional top-level `"page": { "view": "tenders" }` (migrating the Tenders plugin cleanly).
- **Separate portable package from deployment policy:** Following Codex’s `marketplace.json` split, a plugin folder declares *what it contains*; the marketplace/org catalog entry declares *how it is deployed* (`installation`, `authentication`, `privacy`, `execution`, `disabledTools`).
- **Clean break, no compatibility shims:** Breaking changes are allowed across Jaz and the Tenders plugin. Delete legacy `/v1/connections/*`, `connections_search`, ZIP uploads, `skills/sync.go`, `connections/catalog.go`, `remote_mcp.go`, and `integrations.Plugin` outright instead of carrying aliases or dual paths.
- **Same contract for Local and Cloud:** One Go `plugins` domain serves local SQLite/filesystem Desktop and multi-org Postgres/S3 Jaz Cloud.

## Unified Plugin model

~~~jaz-artifact Unified Plugins architecture
<svg width="100%" viewBox="0 0 680 338" role="img">
<title>Unified Plugins architecture</title>
<desc>Hardcoded native integrations, official data-only MCP plugins, and Git marketplaces feed one Plugin Registry. The registry drives the unified Plugins UI and NavRail, resolves effective skills for local ACP or cloud snapshots, routes tools alongside custom MCP servers, and provides read-only script mounts and versioned view state.</desc>
<defs><marker id="p-arr" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="context-stroke" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker></defs>
<g class="c-gray"><rect x="40" y="18" width="184" height="54" rx="8" stroke-width="0.75"/><text class="th" x="132" y="38" text-anchor="middle">Native integrations</text><text class="ts" x="132" y="56" text-anchor="middle">WhatsApp, Telegram, Gmail…</text></g>
<g class="c-gray"><rect x="248" y="18" width="184" height="54" rx="8" stroke-width="0.75"/><text class="th" x="340" y="38" text-anchor="middle">Jaz Marketplace repo</text><text class="ts" x="340" y="56" text-anchor="middle">Tasks, CRM, Zoom, Granola</text></g>
<g class="c-gray"><rect x="456" y="18" width="184" height="54" rx="8" stroke-width="0.75"/><text class="th" x="548" y="38" text-anchor="middle">User / Org Git repos</text><text class="ts" x="548" y="56" text-anchor="middle">Tenders, Base44, private</text></g>
<line x1="132" y1="74" x2="132" y2="108" class="arr" marker-end="url(#p-arr)"/>
<line x1="340" y1="74" x2="340" y2="108" class="arr" marker-end="url(#p-arr)"/>
<line x1="548" y1="74" x2="548" y2="108" class="arr" marker-end="url(#p-arr)"/>
<g class="c-blue"><rect x="40" y="110" width="600" height="62" rx="10" stroke-width="0.75"/><text class="th" x="340" y="133" text-anchor="middle">Unified Plugin Registry (SQLite locally · Postgres + S3 in Cloud)</text><text class="ts" x="340" y="153" text-anchor="middle">Effective set by (org, user, space, bot) · auth accounts · enablement · view state</text></g>
<line x1="112" y1="174" x2="112" y2="210" class="arr" marker-end="url(#p-arr)"/>
<line x1="264" y1="174" x2="264" y2="210" class="arr" marker-end="url(#p-arr)"/>
<line x1="416" y1="174" x2="416" y2="210" class="arr" marker-end="url(#p-arr)"/>
<line x1="568" y1="174" x2="568" y2="210" class="arr" marker-end="url(#p-arr)"/>
<g class="c-gray"><rect x="40" y="212" width="144" height="96" rx="8" stroke-width="0.75"/><text class="th" x="112" y="238" text-anchor="middle">Plugins UI</text><text class="ts" x="112" y="258" text-anchor="middle">Installed and Browse</text><text class="ts" x="112" y="276" text-anchor="middle">Accounts and QR/OAuth</text><text class="ts" x="112" y="294" text-anchor="middle">NavRail and Bots</text></g>
<g class="c-blue"><rect x="192" y="212" width="144" height="96" rx="8" stroke-width="0.75"/><text class="th" x="264" y="238" text-anchor="middle">Skills delivery</text><text class="ts" x="264" y="258" text-anchor="middle">Local ACP paths</text><text class="ts" x="264" y="276" text-anchor="middle">Claude native dir</text><text class="ts" x="264" y="294" text-anchor="middle">Cloud snapshot</text></g>
<g class="c-blue"><rect x="344" y="212" width="144" height="96" rx="8" stroke-width="0.75"/><text class="th" x="416" y="238" text-anchor="middle">Tools and sync</text><text class="ts" x="416" y="258" text-anchor="middle">Deferred MCP proxy</text><text class="ts" x="416" y="276" text-anchor="middle">Per-actor OAuth lock</text><text class="ts" x="416" y="294" text-anchor="middle">Go observe drivers</text></g>
<g class="c-gray"><rect x="496" y="212" width="144" height="96" rx="8" stroke-width="0.75"/><text class="th" x="568" y="238" text-anchor="middle">Assets and views</text><text class="ts" x="568" y="258" text-anchor="middle">Sandbox ro scripts</text><text class="ts" x="568" y="276" text-anchor="middle">Device local MCP</text><text class="ts" x="568" y="294" text-anchor="middle">Versioned view state</text></g>
</svg>
~~~

### Five optional primitives inside any Plugin

Every plugin is identified by a slug (`id`, e.g. `gmail`, `granola`, `crm`, `base44`, `tenders`) and includes whichever of these primitives it needs:

| Primitive | How it is provided | Examples |
| --- | --- | --- |
| **MCP servers / Tools** | Standard `.mcp.json` / `mcp.json` / `mcp_config.json` (Streamable HTTP, SSE, or stdio) **or** a built-in Go driver | `granola`, `zoom`, `tasks`, `crm` (remote MCP via GitHub plugins); `gmail`, `calendar`, `drive`, `slack` (Go driver) |
| **Auth / Accounts** | Standard MCP OAuth discovery (zero manifest config), Go driver OAuth/QR, or declared env/header variables | `granola` (MCP OAuth), `gmail` (multi-account Google OAuth), `whatsapp` (QR device session), `base44` (none) |
| **Skills** | `skills/*/SKILL.md` (+ `references/`, `scripts/`, `assets/`), namespaced `<plugin>:<skill>` | `base44:base44-cli`, `tenders:query-preparation`, `gmail:inbox-triage` |
| **Views & App Pages** | MCP App UI entrypoints (standard `ext-apps`) **or** Jaz HTML `views/*.html` (`page`, `panel`, `inline`) | `tasks`, `crm` (MCP App pages); `tenders` (HTML page + side panels) |
| **Background Sync** | Built-in Go driver implementing `Observe + Materialize` into raw JSONL → `jazmem` `sources/*` | `gmail`, `calendar`, `drive`, `whatsapp`, `telegram` |

### Three orthogonal states

Never conflate **installed**, **enabled**, and **connected**:

1. **Installed / Available:** Built-in plugins and configured marketplace entries appear in **Browse**. Installing a Git plugin pins its commit SHA (in Cloud, org admins set `INSTALLED_BY_DEFAULT` | `AVAILABLE` | `DISABLED`).
2. **Enabled (`bool`):** Toggling a plugin **Off** immediately hides its skills, tools, and NavRail page from agents **without deleting OAuth tokens or saved state**—matching Claude and Antigravity toggles.
3. **Connected (`accounts[]`):** Applies only when a plugin requires sign-in. Clicking **Connect** on an available plugin installs/enables it and runs OAuth or QR sign-in in one step; multi-account plugins (Gmail, Drive, Calendar) support multiple aliases (`work`, `personal`).

### Minimal manifest — zero Jaz boilerplate for standard plugins

A standard connector plugin like **Granola** needs **no `chat.jaz` extension at all**—the exact same folder works in Codex, Antigravity, Cursor, and Jaz:

```text
granola/
├── plugin.json           # name, displayName, description, category, logo, suggestedPrompts
├── .mcp.json             # {"mcpServers":{"granola":{"type":"http","url":"https://mcp.granola.ai/mcp"}}}
└── assets/logo.svg
```

```json
{
  "name": "granola",
  "displayName": "Granola",
  "version": "1.0.0",
  "description": "Search meeting notes and transcripts, and organise them into folders.",
  "category": "Meetings",
  "logo": "assets/logo.svg",
  "suggestedPrompts": [
    "Summarise what we agreed in yesterday's meetings",
    "List open action items from this week's meetings"
  ]
}
```

- **Jaz HTML app pages (e.g. Tenders):** Plugins that ship `views/*.html` simply declare top-level `"page": { "view": "tenders" }` (no `extensions["chat.jaz"]` wrapper and no `$schema` requirement). Update the Tenders plugin manifest and view state to match directly.
- **Where deployment & security policy lives:** On the **marketplace / org catalog entry** (not inside the portable plugin manifest):
  - `policy.installation`: `"INSTALLED_BY_DEFAULT"` | `"AVAILABLE"` | `"DISABLED"`
  - `policy.authentication`: `"ON_INSTALL"` | `"ON_USE"`
  - `execution`: `"cloud"` (default) | `"sandbox"` | `"device"` (e.g. WhatsApp, Blender — badged *"Runs on connected device"*)
  - `privacy`: `"org"` (allowed in shared Team Spaces) | `"personal"` (default **OFF** in Team Spaces unless the acting user toggles it on)
  - `disabledTools`: optional org-admin tool deny-list per plugin

## Multi-ecosystem format compatibility

Third-party repos like [`base44/skills`](https://github.com/base44/skills) already publish manifests for OpenAI Codex, Anthropic Claude, Google Antigravity, and Cursor. **Upstream repos should import into Jaz unmodified.** At Git staging time, one format normalizer maps all four layouts into Jaz’s internal `PluginManifest`:

| Ecosystem | Manifest & marketplace paths | Metadata fields mapped | MCP config format |
| --- | --- | --- | --- |
| **OpenAI Codex** | `.codex-plugin/plugin.json`, `.agents/plugins/marketplace.json` | `interface.displayName`, `interface.shortDescription`, `interface.logo`, `interface.category`, `interface.websiteURL`, `interface.defaultPrompt` | `.mcp.json` / inline `mcpServers` (`url`, `type`) |
| **Anthropic Claude** | `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json` | `name`, `displayName`, `description`, `version`, `author`, `homepage`, `repository`, `skills` | `.mcp.json` / inline `mcpServers` (`url`, `type`) |
| **Google Antigravity / Jaz** | `plugin.json`, `.antigravity-plugin/plugin.json`, `.gemini-plugin/plugin.json` | `displayName`, `logo`, `suggestedPrompts`, `category`, `author`, `homepage`, `repository`, `skills`, `rules/`, optional `page` | `mcp_config.json` (`serverUrl`, `headers`, `variables[]`) or `.mcp.json` / `mcp.json` |
| **Cursor** | `.cursor-plugin/plugin.json`, `.cursor-plugin/marketplace.json` | `displayName`, `logo`, `description`, `author`, `homepage`, `repository`, `skills` | `.mcp.json` / inline `mcpServers` (`url`, `type`) |

### Importer normalization rules

1. **Safe in-repo symlink materialization:** Multi-ecosystem repos symlink shared `skills/` and `assets/` folders across flavor directories (`plugins/base44/*`, `antigravity/base44/skills/*`). Resolve symlinks against the cloned repo root, enforce strict containment inside `repoRoot` (blocking traversal and cycles), and materialize real files into the immutable staged bundle.
2. **Marketplace canonicalization & metadata enrichment:** When a repo ships a `marketplace.json` (`.agents/plugins/` → `.claude-plugin/` → `.cursor-plugin/`), use it as the canonical plugin list (unless an explicit subpath like `antigravity/base44` is configured). When sibling manifests exist for the same plugin name, fill missing UI fields (`logo`, `suggestedPrompts`, `homepage`, `author`) from them so cards are complete regardless of which flavor leads.
3. **Bare skills repo fallback:** If a repo contains no plugin manifest at all, synthesize a plugin from top-level `skills/*/SKILL.md` so community skill repos work out of the box.
4. **Asset & variable normalization:** Accept SVG, PNG, and WebP logos from bare (`assets/logo.svg`) or dotted (`./assets/logo.png`) paths with bounded size and MIME validation. Surface Antigravity `mcp_config.json` `variables[]` as clean user/org config inputs in the plugin detail view.
5. **Hooks & Claude Mods boundary:** Jaz never executes arbitrary plugin hooks or unsandboxed JS/TS Claude Code Mods in `jaz-control` or Cloud. On local Desktop, native Claude Code / Antigravity adapters load enabled plugin directories directly (`--plugin-dir`), so native hooks and Claude Mods continue working inside those harnesses without coupling Jaz’s control plane. Cross-harness UI belongs in sandboxed `views/*.html` or MCP Apps.
6. **ID collisions:** Built-in IDs (`whatsapp`, `telegram`, `gmail`, `calendar`, `drive`, `slack`) are reserved and cannot be shadowed by Git plugins. Installing a second Git plugin with an already-installed `id` from a different source fails with a clear conflict error.

## Sources and marketplaces

### How Browse lists installable plugins

1. **Hardcoded native integrations (`source: "builtin"`, stay in Go):**
   - Only the 6 deeply integrated connectors that rely on custom native protocols, QR sessions, or background `Observe + Materialize` sync (`whatsapp`, `telegram`, `gmail`, `calendar`, `drive`, `slack`) stay hardcoded in Go—never mapped to GitHub repos—and appear directly in the Plugins UI with their existing QR/OAuth/account flows intact.
   - Zero data-only MCP plugins are hardcoded or embedded in the Go binary (`deployink`, `granola`, `zoom`, and `jazapps` are deleted from Go).
2. **Default Official Jaz Marketplace repo (1 GitHub marketplace URL):**
   - Like Claude (`anthropics/claude-plugins-official`) and Codex (`openai/plugins`), Jaz points at **one default official GitHub marketplace repo** containing a `marketplace.json`.
   - Entries point directly to GitHub plugin repos (`git` / `git-subdir`):
     - **Jaz-owned repos:** `tasks` and `crm` live as standard plugins in our own Tasks and CRM repos.
     - **Upstream vendor & community repos:** `zoom` (e.g. `zoom/zoom-plugin-claude`), `granola`, `base44/skills`, Linear, Notion, Salesforce, M365, etc.
   - Adding or updating a plugin in every user’s **Browse** tab is a PR to the marketplace repo—no Jaz release required. The fetched index is cached locally so Browse is instant and works offline.
3. **User & Org Git sources (`source: "git"`):**
   - Configured as `{ repository: "owner/repo", ref: "main", paths?: ["..."], auto_update?: bool }`.
   - **Local Desktop:** Users add GitHub repos/marketplaces in the UI; authors iterate locally with `jaz plugin link /local/path`.
   - **Jaz Cloud:** Org admins configure allowed GitHub repos/paths (private repos via Org GitHub App, e.g. `tenders`), pinned to immutable `commit_sha` and cached once by content digest at `s3://<bucket>/<org_id>/plugins/<id>/<digest>`. No ZIP uploads in Cloud.

User-added custom MCP servers remain separate under **Settings → MCP servers** (`/v1/mcp/servers`), and feed into the same runtime MCP proxy alongside enabled plugins.

### When plugins update from GitHub

Separate **index checks** (safe metadata) from **installed package updates** (immutable commit swaps):

- **Marketplace index refresh (automatic):** Check `git ls-remote` / HTTP `ETag` (zero clone on unchanged SHA) on app startup, when opening **Browse** (debounced, e.g. every 15m), and every **6 hours** in the background. This surfaces new plugins and marks outdated installed plugins with an **Update available** badge.
- **Installed plugin updates (pinned by `commit_sha`):**
  - **Third-party Git repos (`auto_update: false` by default):** Show an **Update** button in the UI rather than silently pulling arbitrary third-party scripts/stdio commands.
  - **Trusted official & org-managed plugins (`auto_update: true`):** Auto-pull on branch updates during background checks—or immediately via a **GitHub webhook** / admin **Sync now** button in Jaz Cloud (e.g. pushing to the Tenders repo rolls out to the org automatically).
  - **Tag or commit SHA pins:** Never auto-update until the user or admin changes the ref.
  - **Zero mid-turn breakage:** Updates stage into a new immutable `<id>@<new_sha>` bundle and swap atomically. In-flight turns finish against the old commit; new turns and reconnected MCP servers pick up `<new_sha>` immediately (and Cloud recompiles `--cloud-skills-snapshot` for the new `catalog_hash`).

## Runtime delivery (Local, Cloud and Bots)

Every turn resolves one **effective plugin set** for `(org, actor, space, bot)`:

- **Tools & Connections:** Enabled plugins feed Jaz’s single deferred MCP gateway (`search_tools`, `call_tool`) plus one discovery tool `plugins_search` (replacing `connections_search`). Calling a tool on a disconnected plugin returns a `jaz://plugin/<id>/connect` card. For upstream MCP servers that do not support OAuth Dynamic Client Registration (e.g. Zoom, Salesforce, M365), server/deployment config supplies pre-registered `oauth_clients` keyed by plugin ID or MCP origin (showing `Setup required` if absent). OAuth refresh is serialized per account (`LockToken` locally, `SELECT ... FOR UPDATE` in Cloud Postgres). In Cloud Team Spaces, `privacy: "personal"` plugins stay off unless the acting user opts them in.
- **Skills:**
  - *Local Jaz:* Exposed at original absolute paths in the ACP prompt catalogue; Claude Code loads enabled plugin directories via session-scoped `--plugin-dir`. Personal loose skills in `~/.jaz/skills/*` keep working alongside plugins on Desktop.
  - *Jaz Cloud:* Compiled into `--cloud-skills-snapshot` keyed by `(org_id, effective_catalog_hash)` for in-memory skill reading. Only plugins containing `scripts/` or `assets/` are bind-mounted read-only at `/jaz/plugins/<id>/` when a workspace sandbox runs.
- **Views & App State:** Replace direct `thread.Folder` disk scans (`os.OpenRoot`) with `plugins.StateStore` backed by `thread_plugin_state(thread_id, plugin_id, data_json, ui_state_json, revision)` in SQLite locally and Postgres in Cloud (with `If-Match` optimistic concurrency on UI saves such as query approvals). Update Tenders to write/read its view state through `show_view` / `StateStore` so `GET /v1/plugins/{id}/page` never touches disk and works with zero sandbox pods awake.
- **Bots integration:** Each bot (`BotDetails`) has a **Plugins** setting—**All enabled plugins** (default) or **Selected plugins** (e.g. `SterlingGreen` bot scopes to `tenders`, `crm`, and `gmail:work`). Keeping bot plugin sets explicit mirrors Claude-style specialized bots and keeps each bot’s skills and tools focused.

## Unified Plugins UI

Merge **Connections** and **Plugins** into one **Plugins** surface modelled after Claude Customize, Codex Plugins, and Antigravity Customizations, while keeping **Settings → MCP servers** as its own separate section:

~~~jaz-artifact Unified Plugins UI mockup
<div style="font-family:var(--font-sans);color:var(--color-text-primary);background:var(--color-background-primary);border:1px solid var(--color-border-tertiary);border-radius:12px;overflow:hidden">
<h2 class="sr-only">Mockup of the unified Jaz Plugins UI combining installed plugins, connections, and a rich detail panel with suggested prompts, tabs, and metadata.</h2>
<div style="display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 16px;border-bottom:1px solid var(--color-border-tertiary)">
  <div>
    <div style="font-size:15px;font-weight:600">Plugins</div>
    <div style="font-size:12px;color:var(--color-text-secondary);margin-top:2px">Connections, skills, tools and app views for Jaz and your bots.</div>
  </div>
  <div style="display:flex;gap:8px;align-items:center">
    <span style="font-size:12px;padding:5px 10px;border-radius:7px;background:var(--color-text-primary);color:var(--color-background-primary);font-weight:500"><i class="ti ti-git-branch" aria-hidden="true"></i> Add from Git</span>
  </div>
</div>
<div style="display:flex;align-items:center;gap:10px;padding:10px 16px;border-bottom:1px solid var(--color-border-tertiary);background:var(--color-background-secondary)">
  <div style="display:flex;gap:4px;background:var(--color-background-primary);padding:3px;border-radius:8px;border:1px solid var(--color-border-tertiary);font-size:12px">
    <span style="padding:3px 10px;border-radius:6px;background:var(--color-background-secondary);font-weight:500">Installed <span style="color:var(--color-text-tertiary)">5</span></span>
    <span style="padding:3px 10px;border-radius:6px;color:var(--color-text-secondary)">Browse <span style="color:var(--color-text-tertiary)">18</span></span>
  </div>
  <div style="flex:1;font-size:12px;color:var(--color-text-tertiary);background:var(--color-background-primary);border:1px solid var(--color-border-tertiary);border-radius:7px;padding:5px 10px"><i class="ti ti-search" aria-hidden="true"></i> Search plugins, skills, tools or accounts…</div>
</div>
<div style="display:grid;grid-template-columns:1.1fr 1.15fr;min-height:310px">
  <div style="padding:12px 14px;border-right:1px solid var(--color-border-tertiary);display:flex;flex-direction:column;gap:8px">
    <div style="font-size:11px;font-weight:600;color:var(--color-text-tertiary);text-transform:uppercase;letter-spacing:.04em">Enabled</div>
    <div style="display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:9px;background:var(--color-background-secondary);border:1px solid var(--color-border-secondary)">
      <span style="width:28px;height:28px;border-radius:7px;background:var(--color-background-warning);color:var(--color-text-warning);display:grid;place-items:center;font-size:14px"><i class="ti ti-layers-subtract" aria-hidden="true"></i></span>
      <div style="flex:1;min-width:0">
        <div style="font-size:13px;font-weight:500">Base44 <span style="font-size:11px;color:var(--color-text-tertiary);font-weight:400">GitHub</span></div>
        <div style="font-size:11px;color:var(--color-text-secondary)">12 skills · base44/skills</div>
      </div>
      <span style="width:28px;height:16px;border-radius:99px;background:var(--color-text-success);position:relative;display:inline-block"><span style="position:absolute;right:2px;top:2px;width:12px;height:12px;border-radius:50%;background:#fff"></span></span>
    </div>
    <div style="display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:9px;border:1px solid var(--color-border-tertiary)">
      <span style="width:28px;height:28px;border-radius:7px;background:var(--color-background-danger);color:var(--color-text-danger);display:grid;place-items:center;font-size:14px"><i class="ti ti-mail" aria-hidden="true"></i></span>
      <div style="flex:1;min-width:0">
        <div style="font-size:13px;font-weight:500">Gmail <span style="font-size:11px;color:var(--color-text-tertiary);font-weight:400">Built-in</span></div>
        <div style="font-size:11px;color:var(--color-text-secondary)"><span style="color:var(--color-text-success)">●</span> work · personal · 9 tools · Sync</div>
      </div>
      <span style="width:28px;height:16px;border-radius:99px;background:var(--color-text-success);position:relative;display:inline-block"><span style="position:absolute;right:2px;top:2px;width:12px;height:12px;border-radius:50%;background:#fff"></span></span>
    </div>
    <div style="display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:9px;border:1px solid var(--color-border-tertiary)">
      <span style="width:28px;height:28px;border-radius:7px;background:var(--color-background-info);color:var(--color-text-info);display:grid;place-items:center;font-size:14px"><i class="ti ti-file-spreadsheet" aria-hidden="true"></i></span>
      <div style="flex:1;min-width:0">
        <div style="font-size:13px;font-weight:500">Tenders <span style="font-size:11px;color:var(--color-text-tertiary);font-weight:400">Org</span></div>
        <div style="font-size:11px;color:var(--color-text-secondary)">2 skills · App page · SterlingGreen</div>
      </div>
      <span style="width:28px;height:16px;border-radius:99px;background:var(--color-text-success);position:relative;display:inline-block"><span style="position:absolute;right:2px;top:2px;width:12px;height:12px;border-radius:50%;background:#fff"></span></span>
    </div>
    <div style="display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:9px;border:1px solid var(--color-border-tertiary)">
      <span style="width:28px;height:28px;border-radius:7px;background:var(--color-background-success);color:var(--color-text-success);display:grid;place-items:center;font-size:14px"><i class="ti ti-brand-whatsapp" aria-hidden="true"></i></span>
      <div style="flex:1;min-width:0">
        <div style="font-size:13px;font-weight:500">WhatsApp <span style="font-size:11px;color:var(--color-text-tertiary);font-weight:400">Device</span></div>
        <div style="font-size:11px;color:var(--color-text-secondary)"><span style="color:var(--color-text-success)">●</span> MacBook Pro · QR connected</div>
      </div>
      <span style="width:28px;height:16px;border-radius:99px;background:var(--color-text-success);position:relative;display:inline-block"><span style="position:absolute;right:2px;top:2px;width:12px;height:12px;border-radius:50%;background:#fff"></span></span>
    </div>
    <div style="display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:9px;border:1px solid var(--color-border-tertiary)">
      <span style="width:28px;height:28px;border-radius:7px;background:var(--color-background-secondary);color:var(--color-text-secondary);display:grid;place-items:center;font-size:14px"><i class="ti ti-plug" aria-hidden="true"></i></span>
      <div style="flex:1;min-width:0">
        <div style="font-size:13px;font-weight:500">Granola <span style="font-size:11px;color:var(--color-text-tertiary);font-weight:400">GitHub</span></div>
        <div style="font-size:11px;color:var(--color-text-secondary)"><span style="color:var(--color-text-success)">●</span> Connected · Meetings</div>
      </div>
      <span style="width:28px;height:16px;border-radius:99px;background:var(--color-text-success);position:relative;display:inline-block"><span style="position:absolute;right:2px;top:2px;width:12px;height:12px;border-radius:50%;background:#fff"></span></span>
    </div>
  </div>
  <div style="padding:14px 16px;display:flex;flex-direction:column;gap:12px">
    <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:8px">
      <div>
        <div style="font-size:16px;font-weight:600">Base44</div>
        <div style="font-size:11px;color:var(--color-text-tertiary);margin-top:2px">base44/skills · 1.0.0-beta.1 · 9a2e41bc</div>
      </div>
      <div style="display:flex;gap:6px">
        <span style="font-size:11px;padding:4px 8px;border-radius:6px;border:1px solid var(--color-border-secondary)">Update</span>
        <span style="font-size:11px;padding:4px 8px;border-radius:6px;border:1px solid var(--color-border-secondary);color:var(--color-text-danger)">Remove</span>
      </div>
    </div>
    <div style="font-size:12px;color:var(--color-text-secondary);line-height:1.45">Build and deploy Base44 full-stack apps: CLI project management, SDK development, and production troubleshooting.</div>
    <div style="border:1px solid var(--color-border-tertiary);border-radius:8px;padding:8px 10px;display:flex;flex-direction:column;gap:5px;background:var(--color-background-secondary);font-size:11px">
      <div><i class="ti ti-arrow-up-right" aria-hidden="true"></i> Create a new Base44 app with a Task entity and deploy it</div>
      <div><i class="ti ti-arrow-up-right" aria-hidden="true"></i> Add a backend function that sends a welcome email</div>
      <div><i class="ti ti-arrow-up-right" aria-hidden="true"></i> Check my Base44 function logs and find why requests fail</div>
    </div>
    <div style="display:flex;gap:6px;font-size:11px">
      <span style="padding:3px 9px;border-radius:99px;background:var(--color-background-secondary);border:1px solid var(--color-border-secondary);font-weight:500">Overview</span>
      <span style="padding:3px 9px;border-radius:99px;color:var(--color-text-secondary)">Skills 12</span>
      <span style="padding:3px 9px;border-radius:99px;color:var(--color-text-secondary)">Bots</span>
    </div>
    <div style="display:flex;flex-direction:column;gap:4px;font-size:11px">
      <div><span style="font-weight:600">/base44-cli</span> <span style="color:var(--color-text-secondary)">Resource config, entities, deploy</span></div>
      <div><span style="font-weight:600">/base44-sdk</span> <span style="color:var(--color-text-secondary)">Client SDK and backend services</span></div>
      <div><span style="font-weight:600">/base44-troubleshooter</span> <span style="color:var(--color-text-secondary)">Function logs and workflow runs</span></div>
    </div>
    <div style="border-top:1px solid var(--color-border-tertiary);padding-top:8px;display:flex;justify-content:space-between;font-size:11px;color:var(--color-text-tertiary)">
      <span>Developer: <strong style="color:var(--color-text-primary);font-weight:500">base44</strong></span>
      <span>Website ↗ · Repository ↗</span>
    </div>
  </div>
</div>
</div>
~~~

### Key UI surfaces

- **Installed | Browse + Search:** `Installed` lists enabled and connected plugins with an inline enable/disable switch. `Browse` shows built-in connectors and subscribed GitHub marketplaces by category with 1-click **Add** or **Connect**.
- **Separate `MCP servers` Settings section:** Kept as it is today for user-added custom HTTP/stdio MCP servers.
- **Unified Detail view (`Overview` | `Skills` | `Tools` | `Accounts` | `Bots`):** Shows only the tabs/sections relevant to that plugin—clickable suggested prompts (starting a new thread), accounts & QR/OAuth management, skill list, tool inspector, pinned sidebar page link, update/remove controls, and developer/version/website/repository metadata.
- **Unified NavRail:** Renders enabled plugins that expose a page/app entrypoint (whether HTML `views/` or MCP global app) from one list, and removes the `+` button from the NavRail. **Plugins** lives in Settings.
- **Onboarding & iOS:** Onboarding reuses `/v1/plugins` filtered to starter plugins; iOS updates to the same `/v1/plugins` response shape in Step 4.

## Backend refactor map

Keep imports strictly one-way (`server` → `httpapi/plugins` → `internal/plugins` → `storage`) and delete duplicated layers outright (no legacy compatibility shims):

| Layer | Responsibility & deletions |
| --- | --- |
| **`backend/internal/plugins`** (unified owner) | Own `Catalog`, multi-ecosystem manifest/MCP normalizer (Codex, Claude, Antigravity, Cursor), default Jaz Marketplace repo sync + cache, Git installer (with safe in-repo symlink materialization, `paths[]`, `git-subdir`), `Enable`/`Disable`, `EffectiveSet(ctx, scope)`, and `StateStore` for views/pages. Delete ZIP upload, `plugin-sources.json`, and `extensions["chat.jaz"]` parsing. |
| **`backend/internal/connections` & `pkg/integrations`** (hardcoded native integrations) | Delete `catalog.go`, `remote_mcp.go`, `integrations.Plugin` metadata types, and all 4 non-native connectors (`deployink`, `granola`, `zoom`, `jazapps`). Keep only the 6 hardcoded Go integrations (`whatsapp`, `telegram`, `gmail`, `calendar`, `drive`, `slack`), their QR/OAuth/token-locking flows, and `Observe + Act + Materialize` primitives, exposed as built-in plugins in the Plugins UI as they are today. |
| **`backend/internal/skills`** | Delete `sync.go` (remote ZIP skill sync). Keep skill frontmatter/loader helpers used by `plugins` and standalone local skills. |
| **`backend/internal/app/mcp.go`** | Simplify `connectionMCPServerReader` into one reader combining user-added custom MCP servers (`store.ListMCPServers`) + enabled plugins’ MCP servers. |
| **`backend/internal/httpapi/plugins`** | Single REST API replacing `/v1/connections/*`: `GET /v1/plugins`, `GET /v1/plugins/{id}`, `PUT /v1/plugins/{id}/enabled`, `POST /v1/plugins/{id}/connect`, `DELETE /v1/plugins/{id}/accounts/{accountId}`, `GET/POST/DELETE /v1/plugins/sources`, `POST /v1/plugins/{id}/update`. Keep `/v1/mcp/servers` untouched for custom MCP servers. |
| **`backend/internal/bots` & `storage`** | Persist `plugin_state` (enabled flag, disabled tools, config variables), `plugin_sources` (Git repos/marketplaces + subpaths + pinned commit), `thread_plugin_state` (versioned view JSON), and optional `bot_plugins` allowlist in SQLite (`jaz`) and Postgres (`jaz-cloud`). |
| **`frontend/src/renderer` & `tenders`** | Delete `ConnectionsSettings.tsx` and `lib/api/connections.ts`; keep `MCPSettings.tsx` as its own Settings section. Ship the unified Plugins hub, detail view, `AddGitSourceModal`, per-bot plugin picker in `BotDetails`, and unified `NavRail`. Update the Tenders plugin manifest and view state to the new contract. |

## Implementation plan and open decisions

### Build order

- [ ] **Step 1 — Unified manifest normalizer & Go cleanup (`backend/internal/plugins`, `backend/internal/connections`):** Support Codex, Claude, Antigravity, and Cursor manifests + `mcp_config.json` + safe in-repo symlinks (drop `extensions["chat.jaz"]`, ZIP upload, and `skills/sync.go`). Expose the 6 hardcoded Go integrations (`whatsapp`, `telegram`, `gmail`, `calendar`, `drive`, `slack`) as built-in plugins; delete `deployink`, `granola`, `zoom`, `jazapps`, `connections/catalog.go`, `remote_mcp.go`, and `integrations.Plugin`.
- [ ] **Step 2 — Official Jaz Marketplace repo & storage (`jaz-plugins` repo, `tasks`/`crm` repos, `storage`):** Publish `tasks` and `crm` plugins in their own repos and create the default Official Jaz Marketplace `marketplace.json` referencing `tasks`, `crm`, `zoom` (upstream), `granola`, etc. Add SQLite tables for `plugin_sources`, `plugin_state`, and `thread_plugin_state`, background `ls-remote`/ETag index refresh, `StateStore`, and `EffectiveSet`.
- [ ] **Step 3 — Unified `/v1/plugins` API, MCP proxy, Bots & Tenders (`httpapi/plugins`, `app/mcp.go`, `bots`, `tenders`):** Delete `/v1/connections/*` routes (keep `/v1/mcp/servers`); wire enabled plugins + custom MCP servers through the MCP reader; replace `connections_search` with `plugins_search`; add `bot_plugins` scoping (defaulting new bots to all enabled plugins); migrate Tenders to the clean manifest and `StateStore`.
- [ ] **Step 4 — Unified Plugins UI & NavRail (`frontend` + iOS):** Replace Settings `Connections` + `Plugins` with the unified Plugins hub (`Installed` / `Browse`, `Add from Git`, rich detail view, toggles) while keeping Settings `MCP servers` separate; add per-bot plugin picker in `BotDetails` and unify `NavRail`.
- [ ] **Step 5 — Cloud storage & snapshot wiring (`jaz-cloud`):** Implement Postgres `org_plugin_sources`, `org_plugins`, `thread_plugin_state`, `connector_tool_catalog`, S3 content-addressed plugin cache, and `--cloud-skills-snapshot` compiler behind the same `plugins` interfaces.

### Decisions

- [x] **Custom MCP servers:** Keep **MCP servers** as its own separate Settings section (`/v1/mcp/servers`, `MCPSettings.tsx`).
- [x] **Bot plugin defaults:** Default every new bot to **All enabled plugins** (with opt-in restriction per bot).
- [x] **Navigation placement:** Keep **Plugins** in Settings and remove the `+` button from `NavRail`.
- [x] **Native vs GitHub plugins:** Keep the 6 hardcoded Go integrations (`whatsapp`, `telegram`, `gmail`, `calendar`, `drive`, `slack`) in Go; serve `tasks`, `crm`, `zoom`, `granola`, etc. as GitHub plugins via the Official Jaz Marketplace repo; remove `deployink`.
