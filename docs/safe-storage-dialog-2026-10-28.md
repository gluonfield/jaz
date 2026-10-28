# Safe Storage dialog, 28 October 16:16 — LEDGER#146

Status: narrow patch on `jaz/unexpected-safe-storage-prompt`, based on main `6c335a55`. Frozen commit, source hashes and evidence hashes are in `runs/safe-storage-dialog/freeze.json`. Awaiting coordinator review/local integration; no main merge, push, release or installed-app action.

## Installed build and remaining caller

At first inspection `/Applications/Jaz.app` was **v0.0.199**, release commit `219543f4`, running as PID 56805 since 16:05:30 UTC. Its Developer ID signature (team `JDDZ55DT74`) passed deep/strict verification and had a stapled notarization ticket. The relevant installed ASAR assets were copied read-only, with SHA-256 hashes in `installed/assets.json`.

The shipped browser path is:

```text
BrowserMenu effect (even when visible=false)
→ preload browserPasswords.state(webContentsId)
→ jaz:browser-passwords → state(target)
→ BrowserPasswordStore.read()
→ safeStorage.isEncryptionAvailable() → Chromium DeriveKey() → Keychain
```

This happens when a side-browser webview becomes ready and on navigation/changed refreshes. It reads the whole password vault **before** filtering to the current origin, so any HTTPS page can trigger it even without a login form or saved login for that site. Automatic login capture independently calls `store.read()` to compare a password before offering Save. Even dismissal returns the same storage-reading state.

The real `browser-passwords.enc` exists (195 bytes; only metadata inspected, modification time 23 October). A new installed browser renderer appeared at 16:16:03; installed logs show a McLaren page at 16:16:10. This strongly matches the screenshot at 16:16:08. There is no retained historical IPC stack proving the exact click; the exact shipped caller is reproduced separately.

The macOS availability method is a key derivation operation, not a harmless permission query. Archived pinned Electron/Chromium source confirms this, including caching failed key derivation for the process. The installed cookie-encryption fuse is disabled (ASCII `48`) in Electron Framework, excluding automatic encrypted-cookie access as this package's equivalent caller. Actual Keychain contents, lock state, ACLs and authorization history were not inspected; the reason macOS required authorization is unknown.

During the task the installed app independently advanced to **v0.0.200**, PID 86719 since 16:37:08. No task command replaced or restarted it. All five relevant main/renderer regions are byte-identical to v199; `installed-v200-comparison.json` records their hashes. The v200 signature also passes deep/strict verification.

## Comparison with prior main fixes

Both v199 and current main contain `aa16fefe` (optional notifications use the resolved auth cache) and `3db31142` (RAM session authentication and Keychain-free local bootstrap). Their ancestry is recorded in `provenance.json`. This is a separate browser password caller left outside those fixes, rather than an absent release of those repairs.

Saved remote restore remains the existing health-first, decrypt-once contract enabled by prior explicit Remember Connection. Ordinary local sessions and notifications need no new credential access. Backend connection persistence and provider-owned Codex/Claude authentication are unchanged.

## Scoped repair

- Browser state and automatic capture read only the vault's memory snapshot. They never read the encrypted file or invoke Safe Storage.
- The Passwords panel shows the site and an explicit **Unlock saved passwords** action, with the “Jaz Safe Storage” explanation visible before clicking. Save explains encryption and possible OS authorization before its click. Import explains both the source browser and Jaz storage before importing.
- The vault owns one lazy, shared process cache. Import and password management use the same instance; construction waits until app identity/profile setup. No separate per-window unlock flags or eager availability probes were added.
- The cache is published after successful encrypted persistence/rename. Import copies the old records before appending, so failed writes cannot alter the active cache. Denied unlock/save leaves previous ciphertext and pending captures intact; refreshes do not retry crypto.
- Existing origin/window/frame checks, encryption provider, Linux `basic_text` rejection, file permissions and ciphertext format remain. No migration, secret deletion, plaintext fallback, key regeneration or trust change.

Cold locked state cannot compare a captured login with saved secrets; it offers Save until the vault is explicitly loaded. A successful unlock keeps the decrypted vault in RAM until process exit. macOS can still require authorization for an intentional protected action. After actual provider denial, the pinned Chromium provider may require restoring access and restarting the app; the synthetic fixture's successful provider is not evidence of same-process native recovery.

Six production files change: net **10 lines**. Shared snapshot ownership removes two passive read/catch paths. The separate regression file exercises production IPC/store/import code, real fixture files, AES-GCM ciphertext and a synthetic Chromium SQLite password database.

## Verification

| Check | Evidence/result |
| --- | --- |
| Exact installed v199 main + BrowserMenu + preload replay, hidden pane | Availability reached on mount, navigation, capture and dismissal; 7 calls across the fixture sequence |
| Final patched ASAR in separate fixture `.app`, `app.isPackaged=true`, Electron 42.3.3 | Denied and allowed runs each have **zero** availability/encrypt/decrypt calls across all passive checkpoints |
| Denied explicit unlock | Exactly one availability call, no decrypt; usable controls, ciphertext preserved; subsequent refreshes add zero calls |
| Allowed explicit unlock/fill/save | One decrypt after click; fill and capture add no crypto; Save encrypts once; a separate validation reader reopens the changed encrypted fixture |
| Native visibility audits | 227 final samples across installed replay and two packaged runs; zero fixture windows on screen or foreground takeovers |
| Normal verification | Full frontend suite: 412 passed before final small review simplifications; final focused six tests/39 assertions pass, including the subsequently added real import/cache regression |
| Static/build checks | Typecheck, desktop production bundle and changed-file lint pass; full lint's test-only missing Bun import was corrected and rechecked |
| Four mutation controls | Eager state read, eager capture read, failed-import cache mutation, and separate importer cache all fail the normal production-path regressions |

The installed replay evaluates unchanged shipped function regions and real React/preload/IPC in an isolated Electron process; it does not launch the installed application. The final fixture `.app` packages the final bundled regions and the same Electron runtime, rather than the entire Jaz startup/backend. All Safe Storage outcomes use an injected synthetic boundary, with `--use-mock-keychain` additionally set. Actual Keychain authorization and Windows/Linux runtime behavior were not exercised.

Evidence: `runtime-summary.json`, `installed-denied/result.json`, `patched-{denied,allowed}/{result.json,visibility.json}`, the packaged logs/screenshots, final tests/build/type/lint logs, `mutation-summary.log`, and archived pinned provider sources in `runs/safe-storage-dialog/`.

No task operation read a Keychain secret, changed a grant/trust setting, generated a real credential prompt, touched real saved credential contents, replaced/restarted the installed app, or altered native agent auth. Real encrypted-file metadata remains unchanged. The independent v200 app update is explicitly recorded rather than claimed to be task-preserved bytes/process state.

## Thermo-nuclear review

Applied `/Users/wins/.jaz/skills/thermo-nuclear-code-quality-review/SKILL.md` to the final diff. No remaining blocker or material finding.

Fixed structural findings: keep snapshots free of I/O instead of hiding an unlock flag in `read`; share the vault across import/management; create it lazily after profile setup; avoid mutating cached records before a write succeeds; build state directly without mutable postprocessing; keep the unlock view open using the existing action contract. There is no dependency addition, backend/transport redesign, new persistence format, unrelated UI polish or file approaching 1,000 lines.

For local integration, review the frozen commit/manifest and merge the task branch through the normal coordinator gate. The installed product receives this behavior only through a later approved package/release; nothing is activated by this handoff.
