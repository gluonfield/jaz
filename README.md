[![License](https://img.shields.io/badge/License-Apache_2.0-D22128?logo=apache&logoColor=white)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Bun](https://img.shields.io/badge/Bun-1.3-000000?logo=bun&logoColor=white)
![Electron](https://img.shields.io/badge/Electron-2B2E3A?logo=electron&logoColor=9FEAF9)
![Claude](https://img.shields.io/badge/Claude-D97757?logo=claude&logoColor=white)
![Codex](https://img.shields.io/badge/Codex-000000?logo=openai&logoColor=white)
[![X](https://img.shields.io/badge/@amgauge-000000?logo=x&logoColor=white)](https://x.com/amgauge)

# Jaz

**all your coding agents, living on the board.**

A personal AI on machines you own — any agent, loops that run overnight, boards,
memory, and git control. Client/server split, always-on, open source end to end.

## Download
[<img width="280" height="60" alt="download-macos" src="https://github.com/user-attachments/assets/1a702cd1-fe55-4ed0-ab99-4e42ca4d4c8f" /><svg xmlns="http://www.w3.org/2000/svg" width="280" height="60" viewBox="0 0 280 60" role="img" aria-label="Download for macOS">
  <rect x="0.5" y="0.5" width="279" height="59" rx="13" fill="#1d1d1f" stroke="#3a3a3c" stroke-width="1"/>
  <g transform="translate(24,15.5) scale(1.2)" fill="#ffffff">
    <path d="M12.152 6.896c-.948 0-2.415-1.078-3.96-1.04-2.04.027-3.91 1.183-4.961 3.014-2.117 3.675-.546 9.103 1.519 12.09 1.013 1.454 2.208 3.09 3.792 3.039 1.52-.065 2.09-.987 3.935-.987 1.831 0 2.35.987 3.96.948 1.637-.026 2.676-1.48 3.676-2.948 1.156-1.688 1.636-3.325 1.662-3.415-.039-.013-3.182-1.221-3.22-4.857-.026-3.04 2.48-4.494 2.597-4.559-1.429-2.09-3.623-2.324-4.39-2.376-2-.156-3.675 1.09-4.61 1.09zM15.53 3.83c.843-1.012 1.4-2.427 1.245-3.83-1.207.052-2.662.805-3.532 1.818-.78.896-1.454 2.338-1.273 3.714 1.338.104 2.715-.688 3.559-1.701"/>
  </g>
</svg>](https://jaz.chat/download)

## Examples

Familiar chat UI

<img width="3358" height="2334" alt="image" src="https://github.com/user-attachments/assets/ab52e8f6-ff3b-450f-895b-027f2faa6481" />

Interactive visualisations

<img width="1280" height="864" alt="ScreenRecording2026-06-20at12 56 26PM-ezgif com-video-to-gif-converter (1)" src="https://github.com/user-attachments/assets/16ef73b3-5525-45f3-86d1-7c16c07cc412" />

Board with live artifacts

<img width="4410" height="2924" alt="image" src="https://github.com/user-attachments/assets/29974b5b-26b2-4f30-aaea-cc4afed6c176" />



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

## Author

**Augustinas Malinauskas** — [@amgauge](https://x.com/amgauge) · [amguage.com](https://amguage.com)

## License

[Apache 2.0](LICENSE).
