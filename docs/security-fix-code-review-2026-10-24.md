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

Extension patch preserves existing WIP. iOS `28d500e` is pushed. Native parity approval and coordinated backend/desktop activation remain open.
