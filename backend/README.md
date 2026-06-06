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

Only `OPENAI_API_KEY` and `OPENROUTER_API_KEY` are read from the environment.

Codex ACP sessions reuse your Codex CLI OAuth login from `~/.codex` by default.
Set `CODEX_HOME` only when Codex uses a non-default auth directory. OpenAI and
OpenRouter credentials only authenticate the coordinator model.

Runtime files are stored under `~/.jaz` by default. Override with `jaz.root`.
