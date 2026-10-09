# App Navigation

- [x] Record CRM and Tasks page visits in Jaz's existing navigation history.
- [x] Restore the app page on Back/Forward, including transitions between apps and chats.
- [x] Forward the same keyboard shortcuts from focused app frames.
- [x] Preserve search-result entry, repeated opens, app state and history replacement.
- [x] Verify real sandboxed apps, run all required checks and perform a thermo-nuclear review.
- [x] Commit verified work; push Tasks and CRM under their standing rules.
- [x] Complete the requested follow-up thermo-nuclear review and verify any corrections.

Contract: opt-in MCP Apps report their app-relative path, push/replace semantics and an optional history delta through `jaz/notifications/navigation`, negotiated as `experimental["jaz/navigation"]`. Jaz keeps the path on its existing app route and restores it through `hostContext["jaz/navigation"]`. Inline apps and file viewers retain their existing behavior.

Verification: the normal hidden Electron browser suite exercises production MCPApps and the titlebar against an independent sandboxed SDK peer. It covers cold restoration, app-owned Back, cross-app Back/Forward, reopens without remounting, tool targets, query restoration, shortcuts and abandoned Forward branches. Disabling host recording makes it fail. The real rebuilt CRM/Tasks apps are also checked against live read-only MCP data.

Review: one existing window history owns every visit; apps opt in through experimental capability negotiation. No extra history stack, dependencies or backend state. The app listener mounts after RouterProvider to preserve TanStack startup; initial default input and late CRM tool results cannot overwrite a restored page.

Checks passed: Jaz full backend suite, 316 frontend tests, typecheck, lint, production bundle and full hidden Electron browser suite; CRM frontend check/build and backend suite; Tasks frontend typecheck/lint/build and backend suite. Tasks `8cd4665` and CRM `1470c7a` are pushed to main. The Jaz branch awaits merge/release.

Follow-up review: no structural blockers found. App frame shortcuts previously checked only typed characters, while Jaz also accepts physical bracket-key codes. The sandboxed navigation regression reproduces the mismatch with non-bracket characters on `BracketLeft`/`BracketRight`; forwarding now uses the same key identity as Jaz's handler. The uncorrected implementation fails the regression. Full backend suite, 316 frontend tests, typecheck, lint, bundle and full hidden desktop suite pass after the fix.
