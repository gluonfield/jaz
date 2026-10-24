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
- [ ] Complete native CLI parity verification: explicit subscription approval requested.
- [ ] Activate backend, desktop and composed extension together; verify the actual tunnel afterward.

The backend and desktop changes are committed in the isolated Jaz worktree. The live backend, tunnel and device configuration remain unchanged while native verification awaits approval. Existing per-device credentials remain valid across the upgrade.

The initial review resolved three contract gaps: pending pairing proofs had bearer authority; read tickets admitted interactive sockets; API CORS intercepted capability-based preview traffic. Device policy now lives in `deviceauth`, encryption in SQLite storage, and the server owns authentication/origin checks. No dependencies or provider prompt/model changes were added.

Validation: 391 frontend tests, typecheck, lint, desktop/web builds; full Go 1.26 suite, vet/build and focused security race checks; real SQLite/HTTP/WebSocket/MCP regressions. A private live-database copy preserved all six connected-account credentials through migration without provider requests. Detailed logs are in `runs/connection-security/`; Live native CLI verification remains the release gate.

- [x] Complete the explicitly requested second thermo-nuclear review; fix reproduced failures and re-verify. See [review findings](security-fix-code-review-2026-10-24.md).
