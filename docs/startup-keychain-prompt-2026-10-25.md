# Startup Keychain Prompt — Coordinator Audit

Status: initial fix `9d1d84f7` merged locally as `b9379ee0`, preserving Stopped/account-card work; its exact-tree integration receipt is in `runs/startup-keychain-prompt/integration/`. The coordinator's optional-notification follow-up is corrected on the v193 main base `f297e93b` and awaiting quick audit; see the final section and `runs/startup-keychain-prompt/notification-cache/freeze.json`. Automatic-connect policy, credentials, browser data and native provider auth are preserved. No further integration, push, tag, release, install, restart, credential migration or signing/trust change is performed by this correction.

## Installed Build and Cause

Inspected `/Applications/Jaz.app` without launching it or reading user credentials. It is v0.0.192, Electron 42.3.3, bundle ID `dev.wins.jaz`, Developer ID team `JDDZ55DT74`, with a stapled notarization ticket. `codesign --verify --deep --strict` passes. The release workflow imports an Apple certificate and builds with electron-builder; the installed artifact confirms the resulting production identity.

Copied only packaged renderer/preload assets into an isolated fixture. Replayed the **installed bytes**, using Electron 42.3.3, synthetic storage and an unavailable safeStorage boundary. The captured renderer stack is:

```text
ThreadNotifications → apiAuthToken → preload.decryptCredential
→ ipcRenderer.sendSync('jaz:credential-encryption')
→ safeStorage.isEncryptionAvailable()
```

`ThreadNotifications` mounts beside the entire app in `main.tsx`. It reads the saved token during render before checking whether notifications are enabled or the connection is verified. Even with notifications disabled and no startup connection preference, a remembered encrypted server credential triggers the boundary. Denial throws out of React render, leaving a blank window. Independently, `verifyBackend` evaluates its default token argument before checking health, reaching the same boundary for an offline saved remote.

On macOS, the synchronous availability check itself derives the encryption key through Keychain. Electron names the service from the application name: `Jaz Safe Storage`, account `Jaz`. Thus the screenshot identifies Jaz's credential-encryption provider; the availability check is not a harmless permission probe. Electron documents that these synchronous calls can block for user input. [Electron safeStorage](https://www.electronjs.org/docs/latest/api/safe-storage), [pinned Electron implementation](https://github.com/electron/electron/blob/v42.3.3/shell/browser/api/electron_api_safe_storage.cc), [pinned Chromium implementation](https://github.com/chromium/chromium/blob/148.0.7778.218/components/os_crypt/sync/os_crypt_mac.mm).

The browser password store only reads on browser/password actions; construction and a missing file do not access safeStorage. The installed executable's `EnableCookieEncryption` fuse is **disabled** (`48`, ASCII `0`), so encrypted Chromium cookies do not explain this package's startup provider access. No fuse is changed.

There was no installed Jaz process running at inspection. This establishes reproducible callers in the exact shipped renderer, not a live stack from the historical screenshot. The actual Keychain item's ACL/history was deliberately not inspected.

## Encryption and Quiet Startup

Desktop safeStorage protects saved backend access tokens (`enc:v1:` envelopes in localStorage) and side-browser saved passwords (`browser-passwords.enc`). Server URLs, labels and launch preferences are ordinary metadata. Backend connector secrets use separate server-side encryption; provider-owned Codex/Claude login stores remain unchanged.

| Desktop platform | Existing protection | Availability policy |
| --- | --- | --- |
| macOS | Application key in Keychain | Fail closed on denied/unavailable access |
| Windows | DPAPI under the Windows user | Fail closed if unavailable |
| Linux | Desktop secret store, such as Secret Service or KWallet | Refuse Electron's `basic_text` fallback |

These are the existing cross-platform code paths and [Electron's documented semantics](https://www.electronjs.org/docs/latest/api/safe-storage). Native Windows/Linux behavior was not run on this Mac.

Keychain access need not ask for a password on every launch. Existing authorization, an unlocked Keychain and consistent application identity permit quiet access. The current installer is correctly signed. Development Electron is ad-hoc, has no signing team and has a different designated requirement, while Jaz still sets the same application name. Sharing that service with a development build is a concrete possible source of a previous-item trust mismatch; it is **not established as this item's history**. No Keychain entry or trust setting was changed.

Claude Code documents macOS Keychain storage. Codex supports file, OS credential-store and ephemeral modes. Their quieter behavior therefore does not establish that they avoid secure storage, nor establish which configuration this user has. Their native storage and fallback policies remain provider-owned. [Claude authentication](https://code.claude.com/docs/en/authentication#credential-management), [official OpenAI authentication documentation](https://developers.openai.com/codex/auth/).

## Scoped Fix

- Notifications configure in an effect after a verified connection; disabled/disconnected notifications do not restore credentials. An optional notification unlock failure disables its monitor without unmounting the app.
- Connection verification checks transport and health before loading a token. Manual remote connection follows the same ordering. Failed local backend startup returns its actual error before reading saved credentials.
- Polling obeys the existing recoverability result. An unavailable credential produces an actionable disconnected state and stops automatic retries; network failures keep retrying.
- Launch actions clear their busy state in `finally` and surface rejected operations, keeping connection controls usable after denial.

Only three production renderer files change: net **18 lines**. Ciphertext format, storage writes, main/preload IPC, Keychain namespace, signatures, native auth, browser passwords and backend code are unchanged. Rebuilt main and preload SHA-256 hashes exactly match the installed package.

## Denial and Recovery Evidence

Evidence directory: `runs/startup-keychain-prompt/` in this worktree. All OS crypto boundaries use synthetic fixtures; no real Keychain operation or provider login ran.

| Final hidden runtime case | Result |
| --- | --- |
| Offline startup, notifications enabled | Zero encryption IPC calls; chooser and all controls available |
| Manual reconnect denied | One unavailable-provider result; error displayed; every connection control enabled |
| Automatic protected reconnect denied | Health precedes unlock; one attempt over seven seconds; actionable chooser, no retry loop |
| Fresh process with fixture access restored | Same ciphertext; token restores once; authenticated check succeeds; onboarding renders |

All four preserve the synthetic saved connection and ciphertext. Native window audits: **278 samples, zero visible fixture windows, zero fixture focus**. Hidden captures were visually inspected. Results and screenshots are in `final-*/result.json`, `final-*/window-audit.json`, and `final-*/window.png`.

The unchanged browser password store was separately exercised against real fixture files with a synthetic AES-GCM provider: nine denied/unavailable read/write/import/remove operations preserve the exact encrypted bytes; restoring the provider recovers the original record. A missing file makes zero provider calls. See `password-evidence.json` and `password-evidence.mjs`.

Final verification: **373 frontend tests pass**, TypeScript checks, focused ESLint, desktop bundle build and `git diff --check` pass. The new regression cases fail against old production source (3 pass / 7 fail). No Go code changed. Main/preload hashes: `58da3c8b…2831b3` / `32d8892c…df517`.

## Required Review and Remaining Boundary

Applied `/Users/wins/.jaz/skills/thermo-nuclear-code-quality-review/SKILL.md` as a self-review. Finding fixed: merely gating credential reads by status still left synchronous IPC in React render; notification setup now owns that side effect and its optional failure. Retained the existing recoverability contract instead of adding a keychain-specific retry flag. No format/API layer, duplicated credential state, new dependency, oversized file or unrelated feature change remains. Final review has no blocking finding in this scoped diff.

This removes the proven unnecessary startup access and denial failures. It **does not guarantee dialog-free restoration of a necessary protected credential when macOS requires new authorization**. The pinned synchronous Chromium provider caches a failed derivation for the process lifetime, so recovery instructions correctly require restoring access and restarting Jaz.

If absolute dialog-free cold launch is required even with denied trust, the concrete next product contract is to defer protected connection restoration to the existing explicit Connect action and leave the chooser available at startup; this trades automatic protected reconnection for a guaranteed quiet launch. Separately, any development/production Keychain namespace separation or trust repair needs a reviewed legacy-decryption plan and must preserve existing ciphertext. Neither broader change is implemented here. The current evidence does not justify replacing keys, resetting entries, weakening storage or changing production signing.

## Coordinator Follow-Up: Optional Notification Credentials

Confirmed the remaining path: healthy `auth_required=false`, old encrypted token still saved, notifications enabled. Connection verification correctly performs no unlock, but the notification effect calls the storage-loading `apiAuthToken`. The earlier regression checked survival after denial and missed the unnecessary call. Its replacement checks zero optional unlocks and verifies the authenticated token explicitly; old source fails only the unauthenticated case (10 pass / 1 fail).

The correction exposes `cachedApiAuthToken` as a read of the API client's existing `authTokens` map and uses it for notification configuration. It performs no storage access or decryption and adds no credential state. Authenticated connection verification and enrollment already populate this cache. The optional unlock/catch branch is deleted: production code is **two lines smaller** overall. Connection verification, launch recovery, persisted data, provider auth and signing are unchanged.

The hidden fixture mounts the production notification component with real React, the production connection store and API client. Its synthetic backend and crypto boundary isolate this optional feature from unrelated application data fetches:

| Hidden case | Credential-provider calls | Final notification configuration |
| --- | --- | --- |
| Before, authentication unnecessary, Keychain unavailable | 1 unnecessary attempt | Disabled after caught denial |
| After, same input | 0; zero credential IPC | Enabled with empty token |
| Before and after, authentication required | 1 by connection verification; no additional notification unlock | Enabled with the verified token |

Both phases preserve the fixture ciphertext. Native audits: 170 before samples and 171 after samples, zero visible/focused fixture windows. The focused gate passes 18 connection/API/notification-monitor regressions, frontend typecheck, changed-file ESLint and whitespace checks. Only the small production-component fixture was bundled; broad suites and full app builds were not repeated. Evidence: `notification-cache/{before,after}-receipt.json`, logs, fixture bundles and native audits; the frozen commit/tree and seals are recorded in `freeze.json` and `manifest.json` there.

Required thermo-nuclear self-review: no remaining findings. The canonical API cache owns resolved credentials; notifications consume it without adding a second cache, persistence mode or policy flag. Removing the independent unlock also removes its exception branch. Necessary protected connection restoration can still require macOS authorization; this correction covers optional notification setup only. Ready for quick coordinator audit, unintegrated.
