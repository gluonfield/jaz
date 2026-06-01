# Jaz Backend

Native Go agent backend with a provider-neutral loop, OpenAI-compatible streaming provider, Codex-style tools, and SSE server.

## Run

```sh
go run ./cmd/jaz serve --provider mock --addr 127.0.0.1:8080
go run ./cmd/jaz chat --server http://127.0.0.1:8080
```

For OpenAI:

```sh
cp application.yaml application.local.yaml
OPENAI_API_KEY=...
APPLICATION_CONFIG=application.local.yaml go run ./cmd/jaz serve --provider openai
```

For OpenRouter, configure the provider explicitly:

```sh
OPENROUTER_API_KEY=...
go run ./cmd/jaz serve --provider openrouter --model openai/gpt-5.4-mini
```

Provider credentials can also live in `application.local.yaml` or `.env`.

Runtime files are stored under `~/.jaz` by default. Override with `jaz.root`.
