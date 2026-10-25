# Installed Device Registration

Status update: diagnosis accepted; `7f4ec279` is held from integration as the final fix. Augustinas requires usable authenticated startup without Keychain persistence. The [session-authentication proposal](session-auth-proposal-2026-10-25.md) awaits independent review; the evidence below is preserved.

The screenshot's generic error hides a failure to save the returned device token securely. macOS logs independently record Jaz Safe Storage ACL rejection for the installed app immediately before the screenshot. The repair reports the secure-save failure while retaining encrypted persistence as a prerequisite for connecting. It does **not** grant Keychain access or claim to cure the installed app's authorization failure.

## Evidence

- Installed `/Applications/Jaz.app`: v0.0.193, bundle `dev.wins.jaz`, Electron 42.3.3. Archive/backend hashes recorded; rebuilt main and preload are byte-identical to the installed copies.
- Desktop log: packaged backend ready at 14:33:31.675Z. Read-only checks validate the existing Ed25519 key pair, public key and SHA-256 device ID. The saved backend device record matches that identity and is approved.
- securityd: installed-app Safe Storage prompts at 14:33:31.685 and 14:33:32.595, ACL rejection at 14:33:32.591 and 14:33:33.170. The logged ACL includes development Electron identities. No Keychain item or trust settings were queried or changed through the Keychain API.
- The historical HTTP response was not logged. Attribution combines those independent logs with the installed-byte reproduction below.

`startLocalBackend` reads the root credential from the announced runtime root's `auth.json` when startup output supplies no key. Trusted IPC returns it to the renderer. `startLocal` tries a saved token, then posts the persistent device profile with that credential to `/v1/devices/register`. The backend's existing local-root policy permits approved registration. The renderer encrypts/persists the returned token, checks `/v1/auth/check`, and connects. Previously, an encryption exception was caught by the outer registration handler and mislabeled as registration failure.

## Isolated Production Verification

Exact installed renderer/preload bytes and main-process bootstrap, identity, permission and encryption regions ran in hidden Electron 42.3.3 windows. The Go 1.26.3 fixture uses the production router, auth middleware, device handler/service and SQLite store from the release source. It starts with an approved synthetic owner; all identities, roots and HTTP ports belong to the fixture.

| Journey | Result |
| --- | --- |
| Installed bytes, storage denied | Registration 200; encryption throws; original screenshot text; no credential persisted or device-token auth check attempted. |
| Installed bytes, storage allowed | Registration 200; protected persistence; auth check 200; onboarding visible. |
| Repaired renderer, denial then allowed retry | Specific secure-save error; stays blocked; retry preserves the identity bytes and reaches authenticated onboarding. |

All three final journeys reject missing/invalid credentials with 401 and forwarded-root/untrusted-origin requests with 403. Across 622 window samples, every fixture window remained hidden and unfocused; no renderer errors occurred. Screenshots were inspected. Installed archive/backend, real identity and root-credential files match their recorded hashes; no real-device mutation or real backend request was performed.

The OS storage boundary is simulated; actual installed-app Keychain authorization and full user-workspace startup remain unverified. Fixture-owned loopback ports replace 5299, main-process regions receive isolated dependencies, and onboarding responses are fixtures. Earlier successful-auth probes had a malformed onboarding fixture; that fixture was corrected before the final three journeys. Full logs, fixture sources, screenshots, hashes and the redacted receipt remain in `runs/installed-device-registration/`.

## Repair And Review

Production change: one narrow catch around `setApiAuthToken`, preserving all registration, identity, trust, cancellation and authentication behavior. Normal-path regressions cover failure without overwriting prior ciphertext, no authenticated access on save failure, recovery with the same identity, and backend rejection without encryption. The new failure regression fails on the old code.

Verification: 375 frontend tests; 20 final focused credential/connection tests; typecheck; focused lint; desktop bundle; Go 1.26.3 device-auth/HTTP suites and focused server security tests. No dependencies or native-agent boundaries changed.

Thermo-nuclear self-review applied `/Users/wins/.jaz/skills/thermo-nuclear-code-quality-review/SKILL.md`. Corrected the error wording to account for both OS encryption and local persistence failures, and expanded newly added one-line conditionals. No remaining structural, ownership, security-policy, duplication or file-size findings: production stays at 763 lines; its test file at 360. The narrow stage boundary earns its four added lines; a new error framework or storage layer would add unnecessary scope.

Separate `cc44de8a` notification-cache changes and restart-continuation work are untouched. The diagnostic commit remains separate from the proposed usable-startup policy. No release, push, installation, restart, device reset, credential migration, Keychain permission change or security-contract change was performed.
