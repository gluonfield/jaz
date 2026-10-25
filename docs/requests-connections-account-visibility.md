# Connections Account Visibility

- [x] Confirm two distinct Gmail account records with display names and OAuth records, using SQLite read-only metadata counts; no identities or token payloads copied.
- [x] Show each account on a separate line in the shared compact provider tile.
- [x] Preserve account search, provider counts, detail controls and the existing connection-card summary.
- [x] Verify and inspect hidden Electron before/after captures at 1280, 390 and 320 CSS pixels.
- [x] Run focused lint, frontend typecheck and thermo-nuclear self-review; commit for coordinator integration approval.

The canonical formatter now exposes account details as a list. Settings renders that list; the in-chat connection card keeps its existing comma-separated summary. Account fallback and MCP server-label rules are preserved. The two-account tile gains exactly 20 CSS pixels, with no horizontal page overflow at the checked widths.

Verification uses the production ConnectionsSettings, tiles and detail modal with synthetic accounts. All six before/after layouts pass: Connected remains 7 providers, searching the second account returns Gmail alone, and opening it shows two Disconnect controls and Add account. The harness blocks backend fetches. Its 197 window samples record zero visible or focused test windows. Typecheck, focused ESLint and git diff whitespace checks pass; full suites were not run for this display-only change.

Thermo-nuclear self-review: no remaining findings. The shared formatting layer owns label selection, array and string consumers retain their contracts, and no new policy branches, dependencies, state or transport changes were introduced. Review caught the existing ConnectCard string consumer; its accountsSummary formatter is retained. Changed files remain 101 and 64 lines. No new permanent tests were added for this reversible layout change.

Synthetic evidence, build sources, metadata counts and logs are in `runs/account-visibility/` in this worktree:

- Desktop: `before-1280.png`, `after-1280.png`.
- Phone widths: `before-390.png`, `after-390.png`, `before-320.png`, `after-320.png`.
- Account controls: `details-390.png`; measurements: `visual-evidence.json`.

Limits: Chromium viewport verification, not physical iOS/Safari testing. Long individual labels retain truncation and a full-text title. Live OAuth sign-in and disconnect were not exercised. The Jaz side browser was unavailable; a standalone hidden Electron harness followed the repository's non-activation pattern. This branch is awaiting coordinator integration approval; the running app and independent task97/task103 worktrees were untouched.
