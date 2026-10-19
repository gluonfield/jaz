# Default Bot Name

- [x] Default new main bots to Prime; adopt an existing Prime when no main exists.
- [x] Keep existing main bots, including Jaz, unchanged. The stored main flag owns identity; no migration is needed.
- [x] Audit bot prompts, roles, Electron copy/onboarding/fixtures, and iOS sources/previews/tests/docs. Bot identity and UI already use saved names.
- [x] Keep app branding, generic voice identity, inactivity copy, and the non-bot message-author fallback as Jaz.
- [x] Update the iOS main-bot fixture and parity documentation on its own branch; leave the main checkout's project edits untouched.
- [x] Verify creation, adoption and existing-install preservation through the real service, ACP manager and SQLite store. Each test fails under its negative control: old default, old adoption name, and forced rename.
- [x] Full backend suite: 102 packages; vet passes. Frontend: 337 tests, typecheck, lint, bundle build and dictation pass.
- [x] iOS build and full suite: 193 tests, three existing skips, zero failures. Window monitor: 1,476 samples, no Simulator windows or focus changes. Original project files, diff and status unchanged.
- [x] Thermo-nuclear review: no blockers. One production constant changes; no new branches, migration, runtime or protocol changes.
- [x] Merge approved; both branches already contained local main and their checks were rerun before local --no-ff integration. Never push; leave unrelated iOS edits untouched.

Merge-check results: frontend remains green. Backend reruns failed in unchanged ACP tests (steering-test directory cleanup, then worktree creation timeout); the steering test passed three focused repeats and the Prime creation/adoption/preservation tests pass. iOS builds pass; full reruns failed in unchanged scroll tests and a URLSession test teardown. The pre-approval full suites passed. iOS project-file hashes, diff and status remain unchanged; both headless monitors recorded zero Simulator windows and focus changes.
