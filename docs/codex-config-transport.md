# Codex launch configuration

Jaz merges its per-process settings and developer instructions with any explicit
`CODEX_CONFIG` override, then writes the resulting JSON to a private temporary
file. The adapter receives `CODEX_CONFIG=@/absolute/path/config.json`, keeping
large memory/skill prompts out of both the adapter and native Codex process
environments. The text is neither shortened nor moved into the user message.

Each running adapter owns a separate file, removed after process exit or a failed
launch. `CODEX_HOME`, its credentials and shared `config.toml` are unchanged.
Explicit overrides accept inline JSON or the same `@file` form.

This transport requires a `codex-acp-app-server` release that supports `@file`.
The managed adapter manifest must pin such a release before this change ships;
command overrides must use a compatible adapter too. Older adapters fail parsing
the reference, rather than silently starting without the configured instructions.
