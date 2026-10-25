# Bot Delegation Briefs

Request: fix bots continuing to write massive delegation prompts after the concise-handoff update.

- [x] Inspect the previous change and an actual handoff after the bot received it.
- [x] Correct guidance for initial handoffs and follow-ups.
- [x] Run focused checks and a thermo-nuclear review.
- [x] Commit the verified change and integrate locally.

Evidence: Meta Challenge Bot `20261025T103221-e834342b` received the current concise-handoff rule at 12:26:28 UTC on 25 October 2026, then authored a 424-word / 3,442-character `create_thread` prompt at 12:28:52. Earlier prompts were 650 and 998 words. The new prompt repeats standing rules and directs a detailed workflow despite the instruction to let the worker choose its approach. This establishes a failure of the existing wording in one observed handoff; it is no measurement of the next wording's effectiveness.

The MCP creation handler passes the authored text through unchanged. Bot identity refresh already supplies changed instructions to existing Codex conversations; the raw native record confirms delivery here. No transport, provider, session or refresh change is indicated.

Change: replace the vague concision rule with a default of one paragraph, usually under 100 words; link existing instructions/state, leave the approach to the worker, and send only changes in follow-ups. Necessary detail remains allowed. No length rejection, truncation or rewriting is added to tool execution.

Validation: Go 1.26.3 focused existing bots/ACP tests pass for configured prompt delivery, home-file guidance, persistence before resume, changed-task notes, identity refresh through failed turns/restarts/compaction and Claude prompt refresh. `git diff --check` passes. Counts and check receipt are in `runs/bot-delegation-briefs/` (gitignored).

Thermo-nuclear self-review: no blocking findings. The change stays at the Jaz-owned bot-guidance boundary, uses the existing refresh contract and adds no runtime logic, helpers, state, dependencies or source-text tests. The word target is guidance with a correctness exception. Existing task ownership, model settings and job-monitoring requirements remain. No native harness, authentication, model metadata, compaction or tool behavior changes.

Limit: these checks protect instruction delivery, not model compliance. No live bot/provider experiment was run; actual handoff brevity still needs observation after activation.

Delivery: code commit `f0a5a98b`, merged into local main as `c632ba78`. The merged tree equals the tested tree (`25b91cd5cfcdf29139c691810bdc69515f477f6e`). No push, release, restart or live bot edits.
