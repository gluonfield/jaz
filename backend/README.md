# Jaz Backend

Go backend for Jaz ACP sessions, model-provider configuration, Codex-style tools, and SSE server.

## Run

```sh
go run ./cmd/jaz
```

Run the server binary directly on the server:

```sh
./jaz --addr :5299 --public-url https://jaz.example.com
```

`jaz serve` and `jaz server` remain compatibility aliases for `jaz`.

Agent defaults are stored in the database and edited from Settings > Agents.
Codex defaults to OpenAI OAuth with `gpt-5.6-sol`; OpenCode defaults to
OpenRouter with `z-ai/glm-5.2`.

Put the OpenRouter key in `.env` or your shell:

```sh
OPENROUTER_API_KEY=...
```

For OpenAI, switch the provider in Settings > Agents and provide:

```sh
OPENAI_API_KEY=...
```

Provider secrets can also come from `.env`: `OPENAI_API_KEY`,
`OPENROUTER_API_KEY`, and `MISTRAL_API_KEY`.

Codex ACP sessions reuse your Codex CLI OAuth login from `~/.codex` by default.
Set `CODEX_HOME` only when Codex uses a non-default auth directory.

The Settings API exposes model-provider endpoint metadata for OpenRouter and
OpenAI. Built-in ACP agent defaults are stored in the same settings record;
Settings controls whether each client is enabled, plus the model and reasoning
effort copied into new threads where the agent supports those fields. Built-in
managed agents such as Codex and Claude do not persist editable launch commands.

On startup, the backend prepares managed ACP adapters from the current Jaz
GitHub release manifest. Release builds fetch `acp-adapters.json` from their
own release tag; local development falls back to the latest release manifest.
The downloaded archives are checksum-verified and installed under `~/.jaz/acp`.
Codex and Claude then launch through those managed binaries.

When developing an unmanaged ACP adapter, add a custom command-based agent in
config and point it at the locally built binary:

```sh
/path/to/codex-acp/target/debug/codex-acp -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"' -c features.tool_search_always_defer_mcp_tools=true -c suppress_unstable_features_warning=true
```

When an ACP agent does not support `session/set_model` or
`session/set_config_option`, clear the unsupported field in Settings > Agents
and pass the setting through that agent's own args or env.

Runtime files are stored under `~/.jaz` by default. Override with `jaz.root`.

For remote Linux deployment and the connected-device direction, see
[`../docs/remote-backend.md`](../docs/remote-backend.md).
