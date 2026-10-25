# Restart continuation investigation

Scope: v193 (`f297e93b`), separate `jaz/restart-agent-continuation` worktree.

- [x] Read prior recovery contract and required thermo-nuclear skill.
- [x] Inspect supplied screenshot and current lifecycle/queue changes.
- [x] Capture bounded read-only production evidence and identify the marker failure.
- [x] Reproduce restart boundaries with fake ACP and production SQLite.
- [x] Write minimal repair design, risks and implementation acceptance checks.
- [x] Complete independent design review before lifecycle implementation.
- [x] Reconcile saved outputs and confirm no interrupted jobs remain.
- [x] Implement approved F2/F4 lineage/finalizer and F3 confirmation persistence correction.
- [x] Add normal-path regressions; run focused Go 1.26 and race checks.
- [x] Complete thermo-nuclear self-review and freeze for independent after-review.
- [ ] Existing independent after-review, before any integration.
- [ ] Automatic ambiguous-delivery recovery: unresolved, task remains open.

Approved implementation scope: F2/F4 durable active-turn lineage and finalizer reason, plus F3 retrying confirmation persistence only on a later qualifying observation. Preserve all no-replay and Stopped guards. No provider work, live model probes, app restart, production edit, integration or release. Ambiguous-delivery automatic recovery remains open.

Accepted diagnostic result: confirmed original work resumed; a second restart interrupted its unconfirmed continuation. Ten repeated-restart diagnostics and 132 existing contract cases passed on Go 1.26.3; removing the safety guard duplicated effects in both negative controls. All 12 original evidence hashes remain unchanged. Independent design review is in `runs/restart-agent-continuation/DESIGN-REVIEW.md`; user accepted the bounded scope in messages 113 and 114. The bounded implementation is complete locally and frozen; independent after-review remains pending. See `restart-agent-continuation-implementation.md` for checks and limitations.
