# Live Bot Status and Smoke Reliability

- [x] Drive bot lists from session and question/permission changes, including owned threads.
- [x] Remove unnecessary list polling; render current state on load without replaying cheers.
- [x] Measure fake-agent status latency before/after, including short runs.
- [x] Diagnose and repair welcome and password smoke failures; measure repeated before/after rates.
- [x] Exercise real behavior with negative controls in hidden, non-activating development Electron; sample windows/focus and capture screenshots.
- [x] Run full backend/frontend checks and thermo-nuclear review; fix findings.
- [x] Stop owned checks and prepare the verified change for commit and approval.
- [ ] After approval: merge local main into this branch, repeat full backend/frontend checks, then clean local main merges this branch with --no-ff. Never push/tag/publish.

Baseline: local main `64ffaea5`, branch `jaz/bots-list-live-and-smokes`. Private measurements and disposable fixtures live in ignored `runs/`.

## Live status

One authenticated bot EventSource per app subscribes to the existing session bus. Filtering happens before buffering, so transcript chunks cannot displace state changes. The server sends a snapshot on every connection, then the affected bot projection when its own thread, worker, routine or group seat changes. The renderer patches the shared query cache and invalidates that bot's activity. No list safety timer was needed: reconnect replaces the snapshot, including changes missed offline.

Removed list polling: panel 3/15 seconds, navigation rail 15 seconds and group chat 2 seconds. The stream's 30-second keepalive carries no data fetch. Activity detail queries retain their existing refresh policy.

Completion timestamps identify successful runs independently of whether React painted the running state. A fresh snapshot establishes the baseline without cheering; only a live completion change animates. Group completion uses its members' turns. A session event after the first prompt is sent also makes Thinking observable immediately.

Measured from a fake-agent command through the real manager, SQLite, HTTP stream and production React hook to the rendered bot list:

| Transition | Before | After |
| --- | ---: | ---: |
| Start → Thinking | 15,000–15,001 ms (3) | 8–25 ms (5) |
| Tool → Working | 3,001–3,009 ms (3) | 8–17 ms (4) |
| Finish → Idle | 3,000–3,008 ms (3) | 11–33 ms (4) |
| Worker question opens / closes | 15,007 / 15,009 ms | 8 / 6 ms |

Final measured median: 9 ms across 15 changes. A short run missed by polling now cheers. Reconnect during work and after offline completion does not replay a cheer; a 16-second idle observation makes zero list requests. Timings are local development measurements, including the command request and frame observation.

## Smoke causes and repeat results

**Welcome:** the history window used to update its visible range in an effect. For an empty chat becoming nonempty, the starter buttons committed one render before the hello. Restoring only the pre-`55598366` history module reproduced the original failure 2/40 times. The baseline already contains that commit's render-time correction; its original welcome check passed 40/40. This change strengthens the normal smoke with a commit-time assertion and waits for the complete hello/starter state. Final strengthened check: 0/40 failures. Restoring the old module now fails deterministically at the split-render assertion.

**Password stage:** the 75-second deadline applied to the whole suite, which takes about 97 seconds; the password stage itself took 1.3 seconds, starting at 84.8 seconds in the final normal run. Original budget: 3/3 full runs timed out. The budget is now 180 seconds, with per-stage timings. A longer-budget intermediate batch exposed a separate tab-readiness race (1/5): Chromium had not created its execution context, and an earlier readiness check could accept about:blank. The smoke now waits for the intended document and retries that transient context error. Profile fixtures also live in each run's user-data directory. Final full suite: 0/5 failures (96–102 seconds), including one normal `bun run test:browser` invocation. These are harness fixes; no password behavior changed.

## Verification and review

- Full uncached backend suite: 102 packages passed; `go vet ./...` passed.
- Frontend typecheck, lint, 337 tests, bundle build and dictation checks passed.
- Nine negative controls failed for the intended reason: transcript filtering, group routing, global subscription, worker ownership, EventSource authentication, welcome split render, missing cheer, reconnect replay and restored polling. Controls ran in copies/overlays.
- Hidden development Electron used `setActivationPolicy('prohibited')`, `show:false`, hidden popups and emulated focus. Window sampler: 12,970 samples over 27 runs, zero test windows; foreground remained the lock screen throughout. Screenshots were captured directly from hidden web contents.
- Full thermo-nuclear review: no remaining blockers. Applied findings: filter before buffering; connect after backend authentication; cancel stale reads and keep completion tracking independent of REST writes; publish the first-prompt transition; preserve group completion; isolate repeated smoke profiles and wait for the correct tab document. Ownership stays in the bot service, transport in its HTTP adapter, cache synchronization in one hook. No dependency, migration or provider-protocol changes; no file crossed 1,000 lines.

Evidence: `runs/verification/{latency-before,live-reviewed,browser-full-final,backend-reviewed,frontend-reviewed}.log`, repeat/control JSON files, `window-audit.json`, and hidden screenshots in the smoke bundles. Main is unchanged pending approval.
