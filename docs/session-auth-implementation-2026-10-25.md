# Session Authentication Implementation

The approved R1–R6 repair makes local authenticated startup and cold restart independent of Keychain persistence. Ordinary credentials and tickets live in RAM. Existing ciphertext stays unchanged; explicit remote remembering still requires secure storage.

## Lifecycle

- Run on This Mac obtains the existing privileged local IPC bootstrap grant, targets exactly `http://127.0.0.1:5299`, rejects redirects, registers the existing identity, and verifies the returned device principal and ID before publication. It never reads the saved old token.
- A nonsecret local launch preference repeats this path after app exit. Existing backend root policy rotates that identity's bearer and may reapprove it. The prior bearer/ticket fail and tracked old requests close. This accepted privileged behavior is distinct from live rejection.
- One actual main-window document owns changes. Main retains one active verified session and revision, distinguishes initial from blocked state, and serializes local issuance across reload/recreation. Board/launcher windows receive only the active device session and verify before mounting API consumers. Exact bundle path, actual webContents/top-level frame, current document and revision guard the handoff.
- Recoverable outages retain the candidate and retry without enrollment or Keychain calls. Authentication rejection clears all app-owned credentials/tickets, request bodies, streams, sockets, query caches and the notification holder. Reload stays blocked until explicit recovery. Ordinary feature 403 responses and failed separate host grants preserve the current device session.
- Missing identity creates the normal first-install identity. Existing malformed, mismatched or unreadable identity fails without replacement.
- Fresh remote invitation, approved bearer or pairing uses the current verification and HTTPS/approval policy and remains session-only. URL history supplies no restart authority. Remember Connection appears only with the existing secure-writer capability and for a connected remote. Successful encrypted restore/save supplies remembering provenance for that current token; reusing it preserves launch intent without another unlock. A fresh/reissued token does not inherit that provenance.
- Remember failure preserves authenticated access and old ciphertext. Remembered remote cold launch remains health-first, decrypt-once and verify-before-cache. Denial stays locked without retries. Disconnect preserves ciphertext; explicit Forget removes only the selected saved credential/history. Client exit/disconnect does not revoke server credentials.

## Independent Review Corrections

The initial implementation was `099c9fbf`. Corrections `6cb3119d` address B1–B4 from `runs/installed-device-registration/SESSION-AUTH-AFTER-REVIEW.md`:

| Finding | Correction and normal-path regression |
| --- | --- |
| B1 | Match existing `unauthorized` / `device_approval_required` error codes only when the rejected request used the active device bearer. Feature errors retain their original response/body. Separate root errors retain the device session. Both actual 401 and 403 authentication failures clear token/ticket/state. |
| B2 | Hydration retries recoverable failures with the verified snapshot retained and candidate private. Owner and secondary regressions show recovery with zero registration and Keychain work. |
| B3 | Keep an unsettled local flight through a remote commit; await it before another local issue. Cancellation after startup/health prevents a registration that has not started. Local → remote → local and reload/document regressions protect ordering. |
| B4 | Preserve restore/save provenance when reusing the same active bearer. Fresh supplied tokens remain session-only. Gate the optional action on secure-writer capability. |

`ffdddaae` includes main through `d33d013b`; auth conflict resolutions retain the separate cache-only notification correction, remove implicit decrypt/migrate from ordinary API reads, and retain both notification-auth regression variants with a valid device response. `82aa189d` includes current main `fd27be87` without further auth conflicts. Restart fixes are preserved.

The final bounded delta extends the B1 regression to 403 and explicitly projects the four permitted handoff fields, with a regression excluding an injected extra root-grant field. This closes runtime shape leakage without widening the session API.

## Verification And Receipt

Evidence root: `runs/installed-device-registration/session-auth/`. Final hashes and check provenance: `freeze.json`; native boundary outcomes: `server-acceptance.json`, five scenario `result.json` files, and `run.log`. Final local merge receipt will be `integration-receipt.json`.

- One frontend suite: 401 passing tests across 73 files. Final focused correction gate: 44 passing tests / 239 assertions across five normal test files. No repeated broad suite.
- Typecheck, full lint, desktop bundle and web build pass. Go 1.26.3 focused production device/auth/HTTP/server security checks pass; backend product code/policy is unchanged by this feature.
- Hidden Electron uses the production main session implementation, production preload, renderer connection/API/stream modules, and isolated production HTTP/auth/SQLite. Its local routing shim confines the fixed production origin to the fixture server. Safe Storage is a denial/encryption boundary fixture; it never calls the real Keychain.
- First local startup: one registration, zero availability/encrypt/decrypt calls, real protected device API/SSE/socket, byte-identical identity and old ciphertext. Owner reload/recreation and concurrent board/launcher do not register again.
- A new Electron process with empty app RAM and the same valid identity performs one cold registration with zero Keychain calls. The old bearer/ticket return 401 and old tracked SSE/socket close. Fixture revocation stays blocked across renderer reload; explicit local recovery uses accepted root reapproval. Outage retries issue no registration.
- Fresh remote authorization requires no Keychain calls. The optional action is visible with a writer and disappears without one. Failed remembering preserves access/ciphertext; successful remembering encrypts once. A new process restores remotely with one decrypt and no registration/local start. Denied saved restore remains blocked, with no decrypt or enrollment.
- The actual bundled full renderer reaches production onboarding after Run on This Mac with authenticated main state and zero Safe Storage calls. Every sampled fixture window stays invisible and unfocused.
- Original installed-v193 denial evidence is preserved. Final read-only hashes confirm installed archive/backend and real identity/auth file are unchanged.

These isolated checks establish authenticated startup, protected requests and lifecycle behavior. Fixture onboarding DTOs do not certify a complete real-workspace session. Safe Storage outcomes are simulated; no real Keychain access was attempted. The installed app remains unchanged. No real device registration/deletion/reset, provider-auth change, release, push, install or restart was performed.

## Thermo-Nuclear Self-Review

Applied `/Users/wins/.jaz/skills/thermo-nuclear-code-quality-review/SKILL.md` to the final feature diff and review corrections. The structural simplification removes implicit credential restore/migration/persistence and duplicated save-before-verify branches, leaving one connection attempt and one bounded main handoff. Connection policy remains in the connection owner; window/document trust and local-flight lifetime live in main; the existing API cache owns headers and transport invalidation. Shared IPC contracts live in `shared`. No new dependency, backend schema/policy, retention framework or provider path was added.

Fixed findings include canceled pre-registration mutation, unsettled-flight loss, outage-as-revocation, feature-error invalidation, remembered-token provenance and runtime handoff-field projection. The original 763-line connection file is 570 lines; main handoff is about 255 and API client 397. No changed feature file crosses 1,000 lines. Remaining branches express the required trust/lifecycle boundaries. No actionable self-review finding remains; independent exact-freeze confirmation is the integration gate.
