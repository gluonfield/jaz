# Installed Device Registration

Source: Augustinas, 2026-10-25; screenshot at 14:33:34 GMT.

- [x] Trace installed v193 renderer, preload, local bootstrap credential, identity and backend registration.
- [x] Reproduce the evidenced failure with isolated production fixtures and installed bytes.
- [x] Add the evidenced error clarification and its meaningful regression coverage; this is diagnostic, not the usable-startup fix.
- [x] Preserve real keys, approvals, credentials, security policy and separate Keychain changes.
- [x] Complete thermo-nuclear self-review and fix findings.
- [x] Commit diagnostic change and evidence as `7f4ec279`; hold it from integration as the final fix.

Restart continuation remains the independent priority. Security contract changes require a proposal before implementation.

Evidence: `runs/installed-device-registration/` (ignored).

Report: [diagnosis, repair, review and verification](installed-device-registration-2026-10-25.md).

## Usable Startup Follow-Up

Source: Augustinas, follow-up and restart recovery, 2026-10-25.

- [x] Accept diagnosis while preserving all original fixture evidence.
- [x] Establish a concrete session-cache/local-bootstrap lifecycle and explicit remembering proposal from production source.
- [x] Coordinator approves the corrected R1–R6 contract in `runs/installed-device-registration/SESSION-AUTH-REVIEW.md`, including privileged cold local recovery.
- [x] Implement only the approved policy; no plaintext disk fallback, provider-auth change or device/credential reset workaround.
- [x] Verify usable authenticated startup with Keychain denied, local cold restart, ciphertext/identity preservation and protected remote restore.

Proposal: [Session Authentication](session-auth-proposal-2026-10-25.md). Implementation and evidence: [approved lifecycle repair](session-auth-implementation-2026-10-25.md). The installed app remains unchanged until a separately authorized activation.

## Approved Implementation

- [x] R1: cold-vs-live local recovery; exact local grant target; live rejection persists across reload.
- [x] R2: pure verification, one guarded token/ticket commit, serialized issuance and cancellation.
- [x] R3: one main-owned RAM snapshot, actual owner/document checks, bounded recipients and reload-safe local flight.
- [x] R4: invalid existing identity fails without rewriting; missing first install remains distinct.
- [x] R5: invalidate every app-owned token/ticket/consumer; preserve ciphertext except explicit Forget.
- [x] R6: explicit secure restore/remember, one remote switcher action, correct auto-connect intent.
- [x] Production-boundary acceptance, thermo self-review, committed freeze for existing independent after-review.

Implementation structure: the existing renderer credential map owns synchronous headers; main retains only the active verified snapshot and lifecycle/revision, with synchronous guarded memory IPC and one asynchronous local bootstrap flight. One renderer connection attempt owns candidate verification/publication. Original evidence is preserved. No backend/device/provider policy change or release/install/restart.

- [x] Correct B1–B4 and integrate current main; preserve notification cache-only and restart fixes.
- [ ] Existing independent delta review approves the exact corrected freeze.
- [ ] Required passing checks plus delta approval → immediate local `--no-ff` merge; no further coordinator pause.
