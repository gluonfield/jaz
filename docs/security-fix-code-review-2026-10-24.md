# Connection Security Code Review

Scope: Jaz `c3a9633c`, iOS `2c9ec63`, extension `302a009`. Applied thermo-nuclear-code-quality-review. All findings fixed.

- **High — stale invitation authority:** bind invitations to the authenticated credential. Rotation now blocks late issuance; delete the duplicate precheck and invalidation loop.
- **Medium — partial credential migration:** encrypt and validate all OAuth rows in one SQLite transaction. A real write failure now rolls back every row.
- **Medium — false connection success:** enrollment and pairing must pass verification before becoming Connected.
- **Medium — cancelled enrollment:** reuse the pairing generation so late responses cannot reconnect after disconnect.
- **Medium — locked-keychain startup:** surface the unlock error through the connection state; preserve ciphertext.
- **Medium — URL policy drift:** normalize the configured CORS origin and share one iOS host parser. Domain names beginning with loopback text use HTTPS.
- **Maintainability:** reuse the trusted-window IPC guard; remove duplicate store lookups and URL parsing.

Regression tests reproduced the first five findings before correction. Verification: full Go 1.26 suite/vet/build, focused race checks, 391 frontend tests/typecheck/lint/builds, 13 iOS tests/build. Hidden simulator checks recorded zero windows/focus.

Extension patch preserves existing WIP. iOS `28d500e` is pushed. Native comparison is complete below. Core merged into main9b46bde7 and backend/desktop activation is verified; the composed extension checkout update and physical-phone installation remain unverified.

Approved native verification completed on the installed runtimes: Codex 0.162.1 / ACP 2.2.2-jaz.3 and Claude 2.1.300 / ACP 0.93.0-jaz.3. Both use subscription OAuth in isolated homes without usable refresh-token copies. Authenticated MCP works initially and after server restart; reload and manual compaction preserve all four markers.

The comparison exposed an existing Codex override that removed native goal tools. Deleted the override; managed-process tests now verify absent, true and false user settings. Opt-in Jaz Goal mode names its MCP server to keep goal ownership explicit. The corrected provider tool definitions and native developer block match exactly. Both arms use GPT-6.1 Sol, low effort and a 475,000-token effective context. First requests contain seven input items (73,298 native / 97,812 Jaz UTF-8 JSON bytes); each first turn makes three provider requests.

Claude uses Opus 5.5 and a 1,000,000-token context. The native system prefix is preserved, and no native tool is missing. Existing CLI/SDK Bash and Monitor definitions differ; SDK core definitions match the previous same-version control. Two dynamic MCP model schemas differ. First requests contain two messages (128,677 native / 157,580 Jaz bytes); both use two main requests, with two SDK auxiliary requests in Jaz. Jaz's system additions and MCP tools explain the larger payloads.

Real login/refresh, physical-phone installation and automatic-compaction threshold timing remain unverified by this bounded security run. Private request/rollout evidence and extracted metrics: `runs/connection-security/native-checks/RESULTS.json`. Subsequent core activation passed local/tunnel authentication and six-account credential migration checks; see the request ledger.
