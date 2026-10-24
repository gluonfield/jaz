# Connection Security Fixes

- [x] Require authenticated MCP and browser connections; restrict browser origins.
- [x] Replace shared root keys with one-use, ten-minute enrollment invitations.
- [x] Close active device sockets and streams on revocation or credential rotation.
- [x] Redeem pairing proof once for a separate device credential.
- [x] Use header authentication for sockets and one-minute tickets for read resources.
- [x] Encrypt OAuth credentials and desktop credential persistence.
- [x] Bind the default listener to loopback and require HTTPS for remote clients.
- [x] Complete the strict maintainability/security review and fix its findings.
- [x] Build, test, commit and push iOS: `28d500e` (13 native tests; physical installation unverified).
- [x] Commit the extension fix: `302a009`; preserve existing checkout changes. The composed patch passes five tests and applies cleanly.
- [x] Complete the approved native CLI comparison: protected MCP, model/context, prompt sizes/counts, native tools, reload and manual compaction. Correct the Codex goal-tool override found by measurement.
- [x] Merge core into main (`9b46bde7`) and activate backend/desktop; verify local and tunnel authentication.
- [ ] Apply the composed extension update to its existing source-checkout WIP; physical-phone installation remains unverified.

The backend and desktop fixes are merged into main and active after the desktop watcher restart. The listener is127.0.0.1:5299. Tunnel health returns200; unauthenticated API/MCP and forged extension-Origin requests return401. Existing per-device credentials remain valid; the nine chats running at backup time resumed.

The initial review resolved three contract gaps: pending pairing proofs had bearer authority; read tickets admitted interactive sockets; API CORS intercepted capability-based preview traffic. Device policy now lives in `deviceauth`, encryption in SQLite storage, and the server owns authentication/origin checks. No dependencies or provider prompt/model overrides were added. The native comparison removes the existing Codex goal-tool override; opt-in Jaz Goal mode explicitly names its MCP server.

Validation before the latest rebase: 391 frontend tests, typecheck, lint, desktop/web builds; full Go 1.26 suite, vet/build and focused security race checks; real SQLite/HTTP/WebSocket/MCP regressions. A private live-database copy preserved all six connected-account credentials through migration without provider requests. Detailed logs are in `runs/connection-security/`; Native verification passed; see the measured results and limits in the review report. Final rebased checks pass: 105 Go packages, vet, 383 frontend tests, typecheck/lint and desktop/web builds. Detailed logs and the pending cutover are recorded in runs/connection-security/RECEIPT.md.

- [x] Complete the explicitly requested second thermo-nuclear review; fix reproduced failures and re-verify. See [review findings](security-fix-code-review-2026-10-24.md).

Live credential migration: all six account records decrypt; refresh/client credentials are preserved. Five records are unchanged, while one access token and expiry refreshed normally. Runtime directory0700; DB/WAL/key0600. The database backup and activated encryption key are kept privately in runs/connection-security/. Plain Python requests met Cloudflare1010; the identified Jaz review client reached the origin and passed the tunnel checks.
