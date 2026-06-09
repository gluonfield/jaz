# Jaz Backend

Native Go agent backend with a provider-neutral loop, native model providers, Codex-style tools, and SSE server.

## Run

```sh
go run ./cmd/jaz serve
go run ./cmd/jaz chat
```

The default provider is OpenRouter. Put the key in `.env` or your shell:

```sh
OPENROUTER_API_KEY=...
```

For OpenAI, set `providers.default: openai` in `application.yaml` and provide:

```sh
OPENAI_API_KEY=...
```

Provider secrets can also come from `.env`:
`OPENAI_API_KEY`, `OPENROUTER_API_KEY`, `ANTHROPIC_API_KEY`, and
`MISTRAL_API_KEY`.

Codex ACP sessions reuse your Codex CLI OAuth login from `~/.codex` by default.
Set `CODEX_HOME` only when Codex uses a non-default auth directory. Native
provider credentials only authenticate the coordinator model.

Native Jaz defaults are stored in the database and edited from Settings >
Agents as the provider, model, and reasoning effort copied into new threads.
The Settings API also exposes hardcoded native provider endpoint metadata for
OpenRouter, OpenAI, and Anthropic. Built-in ACP agent defaults are stored in the
same settings record; Settings controls whether each client is enabled, the
command used to start it, plus the model and reasoning effort copied into new
threads.

For local development with the custom Codex ACP fork built under this workspace,
set the Codex command in Settings > Agents to:

```sh
/Users/wins/Projects/personal/jarvis/codex-acp-zed/target/debug/codex-acp -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
```

When an ACP agent does not support `session/set_model` or
`session/set_config_option`, clear the unsupported field in Settings > Agents
and pass the setting through that agent's own args or env.

Runtime files are stored under `~/.jaz` by default. Override with `jaz.root`.
