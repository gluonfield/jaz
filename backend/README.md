# Jaz Backend

Native Go agent backend with a provider-neutral loop, native model providers, Codex-style tools, and SSE server.

## Run

```sh
go run ./cmd/jaz
```

Run the server binary directly on the server:

```sh
./jaz --addr :5299 --public-url https://jaz.example.com
```

`jaz serve` and `jaz server` remain compatibility aliases for `jaz`.

Native Jaz defaults are stored in the database and edited from Settings >
Agents as the provider, model, and reasoning effort copied into new threads.
The initial database seed uses OpenRouter with `openai/gpt-5.4-mini` and medium
reasoning effort.

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
Set `CODEX_HOME` only when Codex uses a non-default auth directory. Native
provider credentials only authenticate the coordinator model.

The Settings API exposes hardcoded native provider endpoint metadata for
OpenRouter and OpenAI. Built-in ACP agent defaults are stored in the same
settings record; Settings controls whether each client is enabled, the
command used to start it, plus the model and reasoning effort copied into new
threads.

The built-in Codex ACP command uses the Jaz-published adapter:

```sh
npx -y @jazchat/codex-acp@0.16.1 -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
```

When developing the Codex ACP adapter itself, override the Codex command in
Settings > Agents with the locally built binary:

```sh
/path/to/codex-acp/target/debug/codex-acp -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
```

When an ACP agent does not support `session/set_model` or
`session/set_config_option`, clear the unsupported field in Settings > Agents
and pass the setting through that agent's own args or env.

Runtime files are stored under `~/.jaz` by default. Override with `jaz.root`.

For remote Linux deployment and the connected-device direction, see
[`../docs/remote-backend.md`](../docs/remote-backend.md).
