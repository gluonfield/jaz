# Onboarding Permission Steps

- [x] Add permission steps to onboarding, modelled on Meta's Muse onboarding: one slide per job, a card of permission rows with Allow, a check once granted, and "Turned off in System Settings" when refused.
- [x] Cover every macOS permission Jaz uses: Accessibility and Screen Recording (computer use), Microphone (voice and dictation), Notifications (thread finished).
- [x] Run typecheck, lint, tests, build and a rendered check of both slides.
- [ ] Live check on a signed build: each Allow path, the Screen Recording restart resuming onboarding, and notifications.

Design: on macOS desktop the setup order is agents, memory, connections, "Let jaz use your computer" (Accessibility, Screen Recording), "Talk to jaz" (Microphone, Notifications), loops; other platforms skip the two permission slides. The order lives in one `SETUP_STEPS` list that also drives Back/Continue and the progress dots, replacing the hand-linked back/next pointers. Accessibility and Screen Recording Allow open the drag-to-allow panel; Microphone Allow asks through `systemPreferences.askForMediaAccess` and opens the Microphone privacy list once refused; Notifications Allow sends a test notification. Statuses that macOS exposes are polled every 1.5s. A slide with nothing granted offers "Skip for now". The current step is kept in localStorage because granting Screen Recording makes macOS restart Jaz mid-setup.

Notifications: Electron asks macOS for permission when it first creates a notification, and notifications fail until the user answers, so the first test in a process retries for up to 20s ("Answer the macOS prompt") before reporting it refused. Jaz no longer asks at launch; onboarding and Settings ask in context.

Verification: 315 frontend tests, typecheck, lint and the Electron build pass. A standalone build of the two slides with stubbed statuses, captured hidden, rendered the copy, rows, check and Allow states, and both dither motifs.

Dev builds: allowing the terminal (Warp) in Accessibility, Screen Recording and Microphone makes dev read those as granted, since macOS checks the launching terminal. Notifications cannot work in dev: the dev Electron is ad-hoc signed.
