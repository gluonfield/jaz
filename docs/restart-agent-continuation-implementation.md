# Bounded restart repair

Implemented F2/F3/F4 from the independent design review on `jaz/restart-agent-continuation`, based on v193 `f297e93b` and diagnostic commit `e6d03f37`. Independent `AFTER-REVIEW.md` approves frozen `6a4e2a8f` with no blockers; coordinator audit is complete. Local-main integration follows the final Go 1.26.3 backend/vet gate, with its exact tested/merged tree receipt under `runs/restart-agent-continuation/integration/`. **Automatic ambiguous-continuation recovery remains unresolved; task111 stays open.**

## Result

- **F3, local liveness:** confirmation becomes observed only after `SetTurnPromptState` succeeds. A failed write permits another attempt on a later qualifying observation. The existing job lock, sending prerequisite, provider-session/attempt guards and monotonic SQL remain intact. Success suppresses subsequent writes; persistent failure remains unconfirmed. No timer, resend or retry subsystem was added. During an outage there is at most one attempted write per qualifying update/response, rather than a background retry loop.
- **F4, active-turn lineage:** one optional `Turn.ContinuationOf` timestamp travels only through private `sendOptions` and the existing atomic prepared-turn write. Recovery carries an existing root, including through prepared retries, or starts a root from an explicitly confirmed predecessor. Each attempt has a fresh `StartedAt`. Unknown legacy ancestry remains zero; compaction and fresh instructions/claims receive no inherited origin.
- **F2, terminal diagnosis:** the existing unconfirmed sentinel still prevents startup retries. Before the active turn is cleared, the finalizer uses its typed origin to select “Jaz restarted before the agent confirmed the continuation. Earlier task progress is saved.” Parent delivery uses the existing interrupted result path. A duplicate failure for an already-terminal session is a no-op; an interrupted legacy session without a turn can still fail terminally.

Production changes are confined to five existing files: 23 lines added, 7 removed (net +16). No new package, migration, provider branch, protocol field, receipt, daemon, fork, auth or native-runtime change.

## Safety and limits

Sending/unconfirmed original prompts and continuations still receive **no automatic replay**. Root lineage provides diagnosis and never grants recovery permission. Active lineage is cleared by normal terminal completion; this patch preserves the accurate terminal reason and does not add a permanent lineage archive. Stopped ownership continues to gate recovery, dispatch and storage.

The separately audited `runs/restart-agent-continuation/CAPABILITY-ASSESSMENT.md` remains separate work. Native correlation and conditional Codex recovery do not establish safe replay through the current ACP contract. No capability investigation was duplicated here. This authorizes local integration only: no push, tag, release, install, live probe, app restart or production-state edit. Release-time native parity verification remains outstanding.

## Verification

All new regressions run in the normal Go test path. They use fake ACP subprocesses at the external boundary and production transport, SQLite, recovery and finalization. The existing crash fixture now also resumes an existing stored turn; no diagnostic overlay became an inactive product test.

Go **1.26.3** results under `runs/restart-agent-continuation/`:

| Check | Result | Evidence |
| --- | --- | --- |
| Focused affected-package regressions | 46 leaf cases pass | `implementation-focused-go126-final.log` |
| Original sending/action/confirmation-write boundaries | 12 cases pass | `implementation-original-safety.log` |
| Queue claims, unsent prompts and delivered work | 18 cases pass | `implementation-claims-origin.log` |
| Race: repeated restart, legacy/fresh input, failed writes, Stopped, generation guards | 21 cases pass | `implementation-race-go126.log` |
| Race: finalizer for an interrupted legacy session without a turn | Pass | `implementation-finalizer-race.log` |
| Race: final simplified Stopped snapshot fixture | 3 cases pass | `implementation-stopped-final-race.log` |
| Vet | Pass | `implementation-vet-go126.log` |
| F3 negative control: restore the old observed-before-write implementation | Both transient cases fail at the intended assertion: 1 attempt, expected 2 | `implementation-f3-negative-control.log` |

Repeated-restart regressions establish one root through two prepared/recovery boundaries and another confirmed recovery; verify fresh attempt generations; preserve no-replay at sending, received and acted boundaries; retain one parent interruption result and one effect; leave the task open; and keep the accurate error after duplicate finalization. Legacy recovery, fresh input, compaction, native steering, queues and explicit Stopped reactivation retain their contracts.

The first implementation run exposed the duplicate-finalizer reason overwrite, now covered and fixed. A Stopped fixture initially tried to start a stopped turn; the existing database trigger correctly rejected it. The final fixture saves a retained interrupted descendant snapshot through production storage instead. No safety trigger was disabled or weakened.

The original 12 diagnostic evidence files still match `evidence-sha256.json`; the accepted broad experiments were preserved and reused. The two original no-replay negative controls remain accepted evidence against blind replay. No broad experiment was repeated for the implementation.

## Review handoff

Applied `/Users/wins/.jaz/skills/thermo-nuclear-code-quality-review/SKILL.md`; the self-review is `runs/restart-agent-continuation/THERMO-SELF-REVIEW.md`. Exact approved commit/tree, file hashes and evidence bindings are in `runs/restart-agent-continuation/implementation-freeze.json`; independent approval is in `AFTER-REVIEW.md`. Main `1d2b89fe` integrated without conflicts or changes to the reviewed backend/test files. The integration delta review, full backend/vet results and merge receipt are under `runs/restart-agent-continuation/integration/`.
