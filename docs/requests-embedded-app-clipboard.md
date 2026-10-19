# Embedded App Clipboard

- [x] Reproduce a user-activated clipboard write in a hidden Electron window with the MCP app sandbox and permissions policy.
- [x] Grant only `clipboard-sanitized-write` to trusted Jaz windows and their subframes; preserve read, media and font restrictions.
- [x] Cover the permission policy and real embedded, packaged, renderer, untrusted-window and side-browser behavior in the normal frontend checks.
- [x] Verify the old policy fails the new embedded-copy check; five unit controls catch unsafe changes.
- [x] Finish frontend checks, browser smoke, screen sampling and thermo-nuclear review.
- [x] Commit the reviewed branch.
- [x] Inspect Tasks/CRM helpers without modifying those repositories.
- [ ] Obtain approval before merging into local main with `--no-ff`; never push or tag.

The Electron request supplies the containing window, `clipboard-sanitized-write`, `isMainFrame: false` and `requestingUrl: about:srcdoc`. Trust comes from that window's actual URL. The existing policy rejected the permission before considering its origin. Both Electron permission handlers now share the registration used by the app and smoke.

Tasks labels the menu “Copy link” and its toast “Issue URL”; both Tasks and CRM report rejected writes. Jaz's renderer also tries `writeText`, with an `execCommand('copy')` fallback. The side browser uses its separate preview session, which this change leaves untouched.

Verification evidence stays in ignored `runs/clipboard/`. An initial probe's Electron-buffer restore failed its format check; subsequent checks preserve complete native pasteboard items and verify their restoration without printing contents.

Validation: 339 frontend tests, full typecheck/lint and full hidden browser smoke passed. A final app-navigation run also passed with synchronous permission-query checks and native restoration fixes. The screen sampler recorded 3,509 samples with zero test windows or test foreground activity. Native multi-item/custom-format preservation and its no-restore control passed. No backend source changed; no live models were used. Review found no remaining blockers. Other operating systems were not run.
