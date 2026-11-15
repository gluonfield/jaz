# Writing a Jaz plugin

A plugin is a folder installed from Settings → Plugins as a ZIP, or copied into the runtime's `plugins/<name>/` directory. A repository ZIP can contain several plugins. Installing the same name replaces that plugin.

## Manifest

Use [Agent Plugins 1.0](https://agent-plugins.org/specification) with `plugin.json` at the plugin root. `$schema` and `name` are required. The standard metadata fields are `version`, `description`, `author`, `homepage`, `repository`, `license`, and `keywords`. Unknown root fields produce warnings and are ignored; other schema violations reject the plugin.

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
        "view": "tenders",
        "file": "status.json"
      }
    }
  }
}
```

`displayName` labels Settings and the navigation button; it defaults to `name`. `icon` is optional and must name an SVG inside the plugin, starting with `./` and containing no `..` segments. Jaz uses a neutral icon when it is omitted. `page` is optional and contains only `view` and `file`. Top-level `title` and `page` have no Jaz meaning.

```text
tenders/
├── plugin.json
├── assets/icon.svg
├── skills/query-sheet/SKILL.md
└── views/tenders.html
```

## Skills and compatible manifests

Jaz loads each immediate `skills/*/SKILL.md`; it does not search deeper reference or example folders. Skills remain at their installed paths, which are supplied directly to agents.

When root `plugin.json` is absent, Jaz checks `.claude-plugin/plugin.json`, then `.codex-plugin/plugin.json`. An invalid higher-priority manifest is an error. A folder without any of these manifests is not a plugin.

| Manifest | Display name | Additional skills |
| --- | --- | --- |
| Root `plugin.json` | `extensions["chat.jaz"].displayName` | Fixed `skills/` location |
| [Claude manifest](https://code.claude.com/docs/en/plugins/manifest-reference.md) | `displayName` | `skills` path or array of paths, added to `skills/` |
| [Codex manifest](https://developers.openai.com/plugins/build/plugins.md) | `interface.displayName` | `skills` path or array of paths, added to `skills/` |

A declared skills directory can hold `SKILL.md` directly or immediate skill folders. Paths start with `./`; `.` and `./` also name the plugin root. Package paths and symlinks must stay inside the plugin. Missing default component locations are allowed.

Commands, agents, hooks, LSP configuration, other client extensions, and `.app.json` are ignored. Jaz never runs plugin hooks. MCP declarations are retained for subsequent runtime support; this loader does not connect to plugin MCP servers.

## Views and pages

Put HTML views under `views/`. A page's `view: "tenders"` selects `views/tenders.html`; `file` must be a plain filename such as `status.json`.

The navigation page reads that JSON file from working folders known to Jaz threads. Its sandbox receives `window.data = {items}`, where each item has `folder`, parsed `data`, and associated `threads` with `id`, `title`, `status`, and `updated_at`. Folders and threads are ordered by their latest thread update. Missing or invalid JSON is skipped. Subscribe with `window.onData(callback)` for updates.

On a page, `sendPrompt(text)` opens the new-thread composer with the text ready to edit, attach files, and send. `openLink('/sessions/<id>')` opens that thread in Jaz. External links open in the browser.

An agent can also show a view inside a thread using `show_view` with `view: "<plugin>/<view>"`, a `title`, and either `data` for a conversation card or `file` for a side-panel tab. Inside a thread, `sendPrompt` sends to that thread and `openLink` can open its files.
