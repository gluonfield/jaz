# Jaz Backend

Native Go agent backend with a provider-neutral loop, OpenAI-compatible streaming provider, Codex-style tools, and SSE server.

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

Provider secrets and native model settings can also come from `.env`:
`OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_REASONING_EFFORT`,
`OPENROUTER_API_KEY`, `OPENROUTER_MODEL`, `OPENROUTER_REASONING_EFFORT`, and
`MISTRAL_API_KEY`.

Codex ACP sessions reuse your Codex CLI OAuth login from `~/.codex` by default.
Set `CODEX_HOME` only when Codex uses a non-default auth directory. OpenAI and
OpenRouter credentials only authenticate the coordinator model.

Native Jaz model selection stays in `application.yaml`:

```yaml
providers:
  default: openrouter

openrouter:
  model: openai/gpt-5.4-mini
  reasoningeffort: medium
```

Coding-agent models are configured per ACP agent:

```yaml
jaz:
  acp:
    agents:
      codex:
        command: /path/to/codex-acp
        model: gpt-5.5
        reasoningeffort: medium
```

When an ACP agent does not support `session/set_model` or
`session/set_config_option`, remove the unsupported field and pass the setting
through that agent's own `args` or `env`.

Runtime files are stored under `~/.jaz` by default. Override with `jaz.root`.
