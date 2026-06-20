# Jaz

A personal AI assistant platform you run yourself — an always-on backend that drives
coding agents, scheduled loops, durable memory, and live boards, with a desktop app
as its control surface.

> Like a Jarvis you own: the backend holds your sessions, memory, tools, and
> credentials; clients are just windows into it.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

## What it is

Jaz pairs an always-on Go backend with a calm desktop control plane. The backend runs
agent sessions over the [Agent Client Protocol](https://agentclientprotocol.com) (ACP),
recurring loops, durable memory, and MCP tool surfaces. The desktop app lets you watch
live activity, read transcripts, publish boards and widgets, and shape the assistant's
behavior. Run everything on your laptop, or put the backend on a server and connect
clients from anywhere.

## Architecture

Server–client. The backend owns everything; clients are control surfaces.

```
  Desktop (Electron) ┐
  Mobile             ├── HTTP (REST + SSE) ──▶  ┌──────────────────────┐
  CLI                ┘                          │     Backend (Go)     │
                                                │                      │
                                ACP ◀───────────┤  agents              │  owns: sessions,
                Claude · Codex · Grok ·         │  ├─ Claude · Codex   │  memory, tools,
                OpenCode · Jaz (native)         │  ├─ Grok · OpenCode  │  credentials,
                                                │  └─ Jaz (native)     │  workspaces,
                                MCP ◀───────────┤  tools               │  device policy
                  memory · visualise · …        │  └─ memory, …        │
                                                └──────────────────────┘
```

- **Backend** (`backend/`, Go 1.26) — the always-on core. Owns sessions, memory, tools,
  credentials, workspaces, agent configuration, and connected-device policy. Speaks
  REST + Server-Sent Events. Drives coding agents over **ACP** and exposes/consumes
  tools over **MCP**. Runs on a laptop or a Linux server — see
  [`docs/remote-backend.md`](docs/remote-backend.md).
- **Desktop client** (`frontend/`, Electron + React) — the control surface. Streams live
  agent activity, reads transcripts, edits the assistant's identity files
  (`AGENTS.md`, `SOUL.md`), and manages devices and settings. Connects to a local or
  remote backend and stores a per-device token.
- **Agents** — pluggable ACP coding agents (Claude Code, Codex, Grok, OpenCode) plus the
  built-in Jaz agent (OpenRouter / OpenAI).
- **Connected devices** — WhatsApp-style trust: a new desktop, mobile, or CLI client
  requests access and must be approved by an already-trusted device. Per-device bearer
  tokens; Ed25519 device identity.
- **Matrix sidecar** (`matrix/`) — optional server-side Matrix stack for chat/social
  integrations, managed by the server host.

## Features

- **Sessions** — live, streamed agent transcripts.
- **Loops** — scheduled, recurring agent runs.
- **Boards & widgets** — visual artifacts agents publish for you.
- **Memory** — durable markdown memory that persists across sessions.
- **Identity files** — `AGENTS.md` / `SOUL.md` define how the assistant behaves.

## Quick start

Backend:

```sh
cd backend
OPENROUTER_API_KEY=... go run ./cmd/jaz       # listens on :5299
```

Desktop app:

```sh
cd frontend
bun install
bun run dev                                   # JAZ_API_URL=https://… to use a remote backend
```

Server deployment: [`docs/remote-backend.md`](docs/remote-backend.md) ·
Cutting a release: [`docs/RELEASE.md`](docs/RELEASE.md).

Requirements: Go 1.26, Bun 1.3.5, macOS or Linux.

## Screenshots

<!-- Screenshots coming soon. -->

## Author

**Augustinas Malinauskas**

- GitHub: [@gluonfield](https://github.com/gluonfield)
- X: [@amgauge](https://x.com/amgauge)
- Web: [amguage.com](https://amguage.com)

## License

[Apache License 2.0](LICENSE).
