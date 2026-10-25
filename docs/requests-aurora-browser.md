# Aurora Browser Diagnosis

- [x] Inspect thread `20261025T132858-17787445` and native tool results.
- [x] Trace conversation registration and reproduce the failure in Chromium.
- [x] Run focused checks and review the conclusion.

The desktop sends `jaz-auth.<credential>` as its only WebSocket subprotocol (`frontend/src/renderer/src/lib/api/client.ts:133`). The browser handler upgrades without selecting a subprotocol (`backend/internal/httpapi/browser/desktop.go:67`). Chromium rejects this handshake, so the browser never keeps its conversation connection. Opening the panel cannot repair it. Computer Use and Terminal use the same pattern.

Hidden Electron used the production browser handler and isolated SQLite. No subprotocol: opened. Credential subprotocol: error. The raw response was HTTP 101 with no `Sec-WebSocket-Protocol`. Electron 42.3.3 / Chromium 148.0.7778.218; 23 window samples, zero visible/focused. Evidence and reproducible harness: `runs/aurora-browser/` and `backend/runs/aurora-browser/`.

Existing security tests use Gorilla's permissive Go client and explicitly expect no selected protocol, missing Chromium's rejection. Correct by offering and selecting a stable, non-secret protocol alongside the credential; keep authenticating the credential and never echo it.

Go 1.26.3 browsercontrol and browser HTTP tests pass. Read-only diagnosis: no product changes, integration, restart or release. Live renderer network traffic was not captured.
