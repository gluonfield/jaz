[![License](https://img.shields.io/badge/License-Apache_2.0-D22128?logo=apache&logoColor=white)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Bun](https://img.shields.io/badge/Bun-1.3-000000?logo=bun&logoColor=white)
![Electron](https://img.shields.io/badge/Electron-2B2E3A?logo=electron&logoColor=9FEAF9)
![Claude](https://img.shields.io/badge/Claude-D97757?logo=claude&logoColor=white)
![Codex](https://img.shields.io/badge/Codex-000000?logo=openai&logoColor=white)

# Jaz

**all your coding agents, living on the board.**

A personal AI on machines you own — any agent, loops that run overnight, boards,
memory, and git control. Client/server split, always-on, open source end to end.

## Get Started

Grab the latest desktop build from [**Releases**](https://github.com/gluonfield/jaz/releases/latest),
or [run it from source](#development).

Jaz ships with three coding agents, and you pick one per thread:

| Agent | How it connects |
|---|---|
| **Codex** | Sign in with your **ChatGPT Pro plan** to use its discounted tokens, or an OpenAI API key |
| **Claude** | Sign in with your **Claude Pro/Max plan** to use its discounted tokens, or an Anthropic API key |
| **OpenCode** | Connects to OpenAI, OpenRouter, or Anthropic by API token |

You can also connect **OpenRouter**, **OpenAI**, or **Anthropic** directly and pay per token.

## Features

- **Pick the agent per thread** — Codex, Claude, or OpenCode.
- **Loops** — scheduled prompts that run overnight.
- **Boards** — multi-window dashboards of live artifacts.
- **Memory** — survives the thread.
- **Ship from the thread** — built-in git control.
- **MCP servers & skills**, token-usage tracking, and an always-on mode for 24/7 work.

## Architecture

Client/server split — the backend owns everything, clients are control surfaces.

- **Backend** (`backend/`, Go) — the always-on core. Runs agent sessions, loops,
  memory, and tools; owns your credentials and workspaces. REST + SSE; drives agents
  over [ACP](https://agentclientprotocol.com) and tools over MCP.
- **Desktop** (`frontend/`, Electron + React) — the control surface. Runs the backend
  locally, or connects to a remote one over HTTP with a per-device token.

Running the backend on a server: see [`docs/remote-backend.md`](docs/remote-backend.md).

## Development

Requirements: **Go 1.26**, **Bun 1.3.5**.

Backend:

```sh
cd backend
OPENROUTER_API_KEY=... go run ./cmd/jaz   # serves on :5299
go test ./...
```

Desktop app:

```sh
cd frontend
bun install
bun run dev                               # JAZ_API_URL=https://host to target a remote backend
bun run lint && bun run typecheck
bun run build                             # package the desktop app
```

Cutting a release: [`docs/RELEASE.md`](docs/RELEASE.md).

## Screenshots

<!-- Screenshots coming soon. -->

## Author

**Augustinas Malinauskas** — [@amgauge](https://x.com/amgauge) · [amguage.com](https://amguage.com)

## License

[Apache 2.0](LICENSE).
