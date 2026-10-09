# Onboarding Permission Steps

- [x] Add permission steps to onboarding, modelled on Meta's Muse onboarding: one slide per job, a card of permission rows with Allow, a check once granted, and "Turned off in System Settings" when refused.
- [x] Cover every macOS permission Jaz uses: Accessibility and Screen Recording (computer use), Microphone (voice and dictation), Notifications (thread finished).
- [x] Run typecheck, lint, tests, build and a rendered check of both slides.
- [ ] Live check on a signed build: each Allow path, the Screen Recording restart resuming onboarding, and notifications.

Design: on macOS desktop the setup order is agents, memory, connections, "Let jaz use your computer" (Accessibility, Screen Recording), "Talk to jaz" (Microphone, Notifications), loops; other platforms skip the two permission slides. The order lives in one `SETUP_STEPS` list that also drives Back/Continue and the progress dots, replacing the hand-linked back/next pointers. Accessibility and Screen Recording Allow open the drag-to-allow panel; Microphone Allow asks through `systemPreferences.askForMediaAccess` and opens the Microphone privacy list once refused; Notifications Allow sends a test notification. Statuses that macOS exposes are polled every 1.5s. A slide with nothing granted offers "Skip for now". The current step is kept in localStorage because granting Screen Recording makes macOS restart Jaz mid-setup.

Notifications: Electron asks macOS for permission when it first creates a notification, and notifications fail until the user answers, so the first test in a process retries for up to 20s ("Answer the macOS prompt") before reporting it refused. Jaz no longer asks at launch; onboarding and Settings ask in context.

Verification: 315 frontend tests, typecheck, lint and the Electron build pass. A standalone build of the two slides with stubbed statuses, captured hidden, rendered the copy, rows, check and Allow states, and both dither motifs.

Dev builds: allowing the terminal (Warp) in Accessibility, Screen Recording and Microphone makes dev read those as granted, since macOS checks the launching terminal. Notifications cannot work in dev: the dev Electron is ad-hoc signed.

Strict review (2026-10-09): the four permissions had three APIs (`computer.allow`, `microphone.status/allow`, `notifications.test/openSettings`) with the "refused, open Settings" policy split between main and renderer, and three hand-built row UIs. They are now one `systemPermissions` module in main (status as granted/needed/off, one `allow`, notification state kept explicitly instead of an `asked` flag), one `useSystemPermissions` hook and one `SystemPermissionRows` component used by onboarding, Computer Use and Notifications settings. Slides declare their permissions in the slides table, the window kinds come from one shared list, every permission IPC uses one trusted-window check (`trustedWindow`), and panel placement is a pure function. A new IPC test covers the trust boundary, unknown permissions, the status mapping, and the refused microphone and notification paths; removing each guarded branch fails it.

Dev spinner (2026-10-09): in a dev build Notifications Allow spun for 20s under "Answer the macOS prompt" with no prompt, because macOS refuses ad-hoc signed apps silently. Unpackaged builds now report notifications as unavailable ("Needs a signed Jaz build", no Allow), and the release wait for the first prompt is 8s under "Waiting for macOS".
