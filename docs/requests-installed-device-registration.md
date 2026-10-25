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
- [ ] Independent review accepts the persistence-policy change and its token-reissuance/admin-recovery tradeoffs.
- [ ] Implement only the approved policy; no plaintext disk fallback, provider-auth change or device/credential reset workaround.
- [ ] Verify usable authenticated startup with Keychain denied, local cold restart, ciphertext/identity preservation and protected remote restore.

Proposal: [Session Authentication](session-auth-proposal-2026-10-25.md). No persistence-policy implementation is part of this proposal. The user remains blocked; error wording alone is not delivery of usable startup.
