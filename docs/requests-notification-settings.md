# Notification Settings

- [x] Give notifications their own Settings section; the only switch ("Thread finished") was buried in General.
- [x] Let the user request and check the system permission from Jaz, and open Jaz's page in the system's notification settings to configure it.
- [x] Run typecheck, lint, tests, build and a rendered check of the page.
- [ ] Live check: Send test shows macOS's permission prompt on first use, reports blocked when notifications are off, and Open Settings lands on Jaz's notification page.

Design: Settings → Notifications holds a "System notifications" row with Send test and Open Settings, and the "Thread finished" switch moved from General. Electron cannot read the notification permission, so Send test shows a notification: Electron requests macOS authorization on first use, emits `show` when macOS accepts the notification and `failed` when it refuses (Electron 42 `cocoa_notification.mm`). The row reports the result of the last test only. Open Settings deep-links to `com.apple.Notifications-Settings.extension?id=<bundle id>`, reading the bundle id from the running app's Info.plist (`dev.wins.jaz` for the installed app, `com.github.Electron` in dev); on Windows it opens `ms-settings:notifications`. Notifications have no drag-to-allow list in System Settings, so the permission guide panel does not apply.

Verification: 315 frontend tests, typecheck, lint and the Electron build pass. The bundle-id lookup returned the right id from both real Info.plist files. A standalone build of the page, captured hidden, rendered the initial, allowed (dark) and blocked states correctly.
