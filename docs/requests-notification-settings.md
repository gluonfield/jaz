# Notification Settings

- [x] Give notifications their own Settings section; the only switch ("Thread finished") was buried in General.
- [x] Let the user request and check the system permission from Jaz, and open Jaz's page in the system's notification settings to configure it.
- [x] Run typecheck, lint, tests, build and a rendered check of the page.
- [x] Live check in dev: Send test reported blocked and Open Settings opened the Notifications overview.
- [x] Diagnose: the dev Electron.app is ad-hoc signed (no team), so macOS refuses its notification requests without a prompt and never lists it; the installed Jaz is Developer ID signed (team JDDZ55DT74). The deep link used the extension form, which reportedly ignores `?id=`; switched to `com.apple.preference.notifications?id=`. Electron asks macOS for permission only when it first creates its notification presenter, so the first notification raced the prompt. Superseded by onboarding: Jaz asks in context, and the first test retries while the prompt is open (see requests-onboarding-permissions.md).
- [ ] Live check on a signed build: macOS asks at first launch, Send test delivers or reports blocked, and Open Settings lands on Jaz's page.

Dev builds cannot exercise system notifications: they are ad-hoc signed.

Design: Settings → Notifications holds a "System notifications" row with Send test and Open Settings, and the "Thread finished" switch moved from General. Electron cannot read the notification permission, so Send test shows a notification: Electron requests macOS authorization on first use, emits `show` when macOS accepts the notification and `failed` when it refuses (Electron 42 `cocoa_notification.mm`). The row reports the result of the last test only. Open Settings deep-links to `com.apple.preference.notifications?id=<bundle id>`, reading the bundle id from the running app's Info.plist (`dev.wins.jaz` for the installed app, `com.github.Electron` in dev); on Windows it opens `ms-settings:notifications`. Notifications have no drag-to-allow list in System Settings, so the permission guide panel does not apply.

Verification: 315 frontend tests, typecheck, lint and the Electron build pass. The bundle-id lookup returned the right id from both real Info.plist files. A standalone build of the page, captured hidden, rendered the initial, allowed (dark) and blocked states correctly.

Strict review (2026-10-09): Send test and Open Settings became the shared system permission row. Allow sends a test notification; once macOS refused one, Allow retries once and otherwise opens Jaz's notification settings. The Windows settings link was dropped with the rest of the macOS-only permission UI.
