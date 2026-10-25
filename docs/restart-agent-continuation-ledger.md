# Restart continuation investigation

Scope: v193 (`f297e93b`), separate `jaz/restart-agent-continuation` worktree.

- [x] Read prior recovery contract and required thermo-nuclear skill.
- [x] Inspect supplied screenshot and current lifecycle/queue changes.
- [x] Capture bounded read-only production evidence and identify the marker failure.
- [x] Reproduce restart boundaries with fake ACP and production SQLite.
- [x] Write minimal repair design, risks and implementation acceptance checks.
- [ ] Complete independent design review before lifecycle implementation.

No lifecycle changes, model calls, app restart, live-state repair, release or integration at this stage. Provider behavior and authentication stay provider-owned.

Result: confirmed original work resumed; a second restart interrupted its unconfirmed continuation. Recovery replaced the original confirmation, and the next startup emitted the first-task error. Ten repeated-restart diagnostics and 132 existing contract cases pass on Go 1.26.3; removing the safety guard duplicates effects in both negative controls. See `restart-agent-continuation-design.md`. Independent review is pending; lifecycle implementation has not started.
