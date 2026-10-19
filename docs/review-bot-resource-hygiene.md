# Bot Resource Hygiene Review

Archived bot workers participate in managed-worktree pruning. Every recorded thread sharing the directory must be eligible, and live processes retain it. Unarchived bot work stays outside the ordinary retention cap. Completed workers and descendants may archive after their commitment has been closed for seven days and they have been inactive for seven days; pinned, queued, interrupted and background-active work remains. Workers without a closed commitment require explicit archival. Unowned directories are outside automatic pruning.

Removal preserves conversations, branches and tracked/untracked source through the existing Git snapshot operation. Ignored files are discarded on removal. Detached or unexpected branches and reflog-only commits block removal. ACP resume restores the managed directory through its recorded owner, including for reviewers in nested directories.

Normal idle limits remain ten minutes for threads and one hour for Claude bot processes. Memory pressure permits one additional stop per one-minute sweep, oldest eligible process first, after at least one minute idle. Existing turn, lease, question, background-task and provider-subagent guards remain. macOS reads kernel warning/critical flags; Linux uses `MemAvailable / MemTotal < 10%`. Failed readings retain normal policy. Early stopping trades memory for a cold resume and possible Claude prompt-cache rewriting.

## Thermo-Nuclear Review

Five findings repaired:

| Finding | Resolution |
| --- | --- |
| Per-thread pruning could remove a directory still used by a reviewer. | Group by managed directory, lock all owners against resume and recheck persisted eligibility. |
| Reviewers could restore from their own slug instead of the shared worktree's branch. | Resolve the recorded owner for all restoration paths. |
| A branch-backed HEAD could hide commits retained only in the worktree reflog. | Refuse removal until those commits have durable refs. |
| A config-process fixture could read a newly created, empty receipt. | Wait for nonempty contents; production configuration is unchanged. |
| Relative workspace configuration could mismatch stored absolute paths. | Resolve the managed root before the archival query. |

Approved after fixes. Pruning policy belongs in `internal/worktrees`, archival eligibility stays in transactional storage, and process retirement reuses existing runtime guards and locks. No dependency, scheduler, persistent policy flag, migration or provider protocol was added. No handwritten file crosses 1,000 lines.

## Verification

- Full Go 1.26 backend suite and vet passed. Pressure behavior passed 20 plain repetitions and three race-enabled repetitions; the receipt fixture passed 30 repetitions.
- Fourteen build-overlay negative controls failed for their intended defects, covering pressure selection, lease/process guards, shared/nested restoration, retention scope, reflog preservation and archival eligibility.
- An isolated real Codex session retained normal idle behavior, stopped under a simulated OS pressure reading, then resumed the same native conversation with recall and recovered source changes. Shared-directory removal required both threads to be archived. A copied worktree also exercised production pruning and restoration.
- Linux/amd64 ACP tests cross-compiled; Linux runtime execution remains unverified because Docker was unresponsive. Platform-independent Linux parser cases pass in the normal suite.

The live probe replaced only the OS memory reader and allocated no actual memory pressure. It verifies stop/resume continuity; provider prompts, model metadata, tools, compaction and authentication code are unchanged.
