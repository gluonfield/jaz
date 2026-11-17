# Writing a Jaz plugin

A plugin is a folder installed from Settings → Plugins using a Git repository or ZIP, or copied into the runtime's `plugins/<name>/` directory. A repository can contain several plugins. Installing the same name replaces that plugin.

## Repository installation and updates

Choose **Add plugin**, enter GitHub `owner/repo` or a Git URL, and optionally choose a branch, tag or commit. Standard HTTPS, SSH, file URLs and backend-local repository paths use the backend machine’s existing Git access. Git runs without interactive credential prompts or checkout hooks; URLs containing passwords or query parameters are rejected. Private repositories must already be accessible to that machine. **Choose ZIP** and ZIP drops remain available.

Jaz reads `.agents/plugins/marketplace.json` first, then `.claude-plugin/marketplace.json`. Local entry sources are `"./plugins/example"` or `{ "source": "local", "path": "./plugins/example" }`, relative to the repository root. Unsupported `github`, `url`, `git-subdir`, `npm`, `archive` and `command` entry sources are skipped and reported by name. Other marketplace metadata is ignored. Without a marketplace, Jaz discovers plugin folders throughout the exported repository as it does for ZIP archives. Git submodules and symbolic-link archive entries are not installed.

Repository, ref and resolved commit appear beside each plugin’s version. **Update** fetches that source and replaces only that plugin. Source records live in the runtime’s `plugin-sources.json`, outside plugin folders. ZIP replacement clears that plugin’s source and Update action.

Packages are fully staged and validated before publication. Replacement moves the previous package aside, publishes the new package and source, and restores the previous package on failure. Store readers see coherent package/source records. Installation, update and removal respond after publication and refresh the changed MCP servers in the background, so changed code reconnects even when its server config stays the same. Settings status and App entrypoints refresh within five seconds once connected.

## Native agents

The bundled managed Claude adapter loads installed folders for each new or restored session through Claude Code's session-scoped plugin flags. Running sessions keep their startup folder selection. Claude owns their hooks, commands, agents and native skills; Jaz never runs hooks or interprets those other components. Plugin MCP servers remain on Jaz's proxy and are excluded from Claude's native discovery. No native profile or global plugin configuration is changed.

Only skills selected for native loading are omitted from Jaz's generated Claude catalogue. The composer keeps the complete `plugin:skill` catalogue and original paths, and other providers keep Jaz's catalogue. Native Claude uses `.claude-plugin/plugin.json`'s name, or the installed folder name without that manifest. An unreadable, malformed or conflicting Claude overlay produces a warning on that plugin and keeps its skills in Jaz's catalogue; other plugins still load natively. Custom/remote Claude adapters retain Jaz skill ownership because their startup support is unknown. Codex currently has no supported session-scoped folder option, so Jaz does not install plugins into its native configuration.

Restoring a Claude conversation passes the current native folder selection without rewriting its provider history. Older prompt snapshots can retain the previous Jaz catalogue until native compaction rebuilds them.

## Manifest

Use [Agent Plugins 1.0](https://agent-plugins.org/specification) with `plugin.json` at the plugin root. `$schema` and `name` are required. The standard metadata fields are `version`, `description`, `author`, `homepage`, `repository`, `license`, and `keywords`. Unknown root fields produce warnings and are ignored. Jaz checks the schema identifier, plugin name, metadata types, and supported Jaz fields.

Jaz settings belong only under `extensions["chat.jaz"]`:

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "tenders",
  "version": "1.0.0",
  "description": "Prepare tender query sheets",
  "extensions": {
    "chat.jaz": {
      "displayName": "Tenders",
      "icon": "./assets/icon.svg",
      "page": {
        "view": "tenders"
      }
    }
  }
}
```

`displayName` labels Settings and the navigation button; it defaults to `name`. `icon` is optional and must name an SVG inside the plugin, starting with `./` and containing no `..` segments. Jaz uses a neutral icon when it is omitted. `page` is optional and contains only `view`. Unknown fields in `chat.jaz` and its `page` produce warnings and are ignored, including the obsolete `page.file`. Top-level `title` and `page` have no Jaz meaning.

```text
tenders/
├── plugin.json
├── assets/icon.svg
├── skills/query-sheet/SKILL.md
└── views/tenders.html
```

## Skills and compatible manifests

Jaz loads each immediate `skills/*/SKILL.md`; it does not search deeper reference or example folders. Plugin skills are named `<plugin>:<skill>` in agent catalogues, `$` mentions and Settings, such as `tenders:query-sheet`. Skills remain at their installed absolute paths, which are supplied directly to agents. Standalone skills keep their plain names.

When root `plugin.json` is absent, Jaz checks `.claude-plugin/plugin.json`, then `.codex-plugin/plugin.json`. An invalid higher-priority manifest is an error. A folder without any of these manifests is not a plugin.

| Manifest | Display name | Additional skills |
| --- | --- | --- |
| Root `plugin.json` | `extensions["chat.jaz"].displayName` | Fixed `skills/` location |
| [Claude manifest](https://code.claude.com/docs/en/plugins/manifest-reference.md) | `displayName` | `skills` path or array of paths, added to `skills/` |
| [Codex manifest](https://developers.openai.com/plugins/build/plugins.md) | `interface.displayName` | `skills` path or array of paths, added to `skills/` |

A declared skills directory can hold `SKILL.md` directly or immediate skill folders. Paths start with `./`; `.` and `./` also name the plugin root. Package paths and symlinks must stay inside the plugin. Missing default component locations are allowed.

Commands, agents, hooks, LSP configuration, other client extensions, and `.app.json` are ignored. Jaz never runs plugin hooks. Plugin MCP servers run through Jaz’s managed MCP proxy and expose their tools to agents. MCP Apps reuse Jaz’s existing app frames.

## MCP servers

Portable plugins use root `mcp.json`:

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "validator": {
      "type": "stdio",
      "command": "./bin/validator",
      "args": ["--data", "${PLUGIN_DATA}"],
      "cwd": "${PLUGIN_ROOT}"
    }
  }
}
```

Jaz supports `stdio`, `streamable-http`, and legacy `sse`. A stdio command is a bare executable name or a contained `./` package path; no shell is inserted. The working directory defaults to the plugin root. `${PLUGIN_ROOT}` and `${PLUGIN_DATA}` expand once in arguments, environment values and working directories; other placeholders remain literal. Commands, environment keys, remote URLs and headers are not expanded. The reserved variables are also supplied to subprocesses. Persistent writable data lives in the runtime’s `plugin-data/<name>/` directory, outside the replaceable package, and survives removal.

Portable remote endpoints require HTTPS except for loopback hosts. Configured headers stay on the declared origin and cannot override protocol or authorization headers generated by the client. Streamable HTTP uses Jaz’s existing OAuth sign-in. Legacy SSE supports configured headers; interactive SSE OAuth sign-in is not supported.

Claude/Codex layouts load `.mcp.json`, then manifest `mcpServers` JSON paths or inline maps in order. Later names replace earlier ones. Legacy `type: "http"` means Streamable HTTP. These configs expand `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}`, environment references and `${VAR:-default}` in commands, arguments, environment values, URLs and headers. MCP bundles and executable credential helpers are not loaded.

Settings → Plugins lists server names and per-server configuration diagnostics. Derived servers are managed by their plugin; edit or remove the package to change them. ZIP install/replacement/removal refreshes tools and MCP Apps before the request completes. Direct filesystem changes are read on the next MCP reconnect or server restart. Configurations are not copied into saved MCP settings. An invalid server does not prevent the plugin’s other components from loading.

## Views and pages

Put HTML views under `views/`. A page's `view: "tenders"` selects `views/tenders.html`.

The navigation page includes only threads that have called `show_view` with a view of that plugin and a `file`. For each thread, Jaz follows its latest such presentation, including archived threads. The file must be a plain filename inside the thread's working folder; subdirectories, traversal, and symlinks outside that folder are rejected. An incidental file with the same name does not link a thread. Older stored presentations without a plugin/view reference still render, but do not link a thread to a page. Its sandbox receives `window.data = {items}`, where each item has `folder`, parsed `data`, and associated `threads` with `id`, `title`, `status`, and `updated_at`. Items group threads by both absolute folder and filename. Items and their threads are ordered by the latest thread update. Missing or invalid JSON is skipped. Subscribe with `window.onData(callback)` for updates.

On a page, `sendPrompt(text)` opens the new-thread composer with the text ready to edit, attach files, and send. `openLink('/sessions/<id>')` opens that thread in Jaz. External links open in the browser.

An agent can also show a view inside a thread using `show_view` with `view: "<plugin>/<view>"`, a `title`, and either `data` for a conversation card or `file` for a side-panel tab. Inside a thread, `sendPrompt` sends to that thread and `openLink` can open its files.


File-following plugin views are restored when a thread is reopened. The titlebar's **Plugin views** button reopens any shown view without finding its transcript row.

Inside a file-following thread view, `openView('quiz', 'Tender quiz')` opens another HTML view of the same plugin using the caller's same data file. The name is a plain view filename without `.html`, never a plugin reference or path. Each plugin/view/file combination has one tab; reopening unchanged HTML focuses that tab without adding another transcript row. Changed HTML saves a new presentation. Retained views follow file updates through `onData` while work runs, when it ends, and when a tab becomes visible. Legacy views without an owner reference still render but cannot call `openView`. This bridge is not available on navigation pages or ordinary artifacts.
