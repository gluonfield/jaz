# Bot Message Visibility and Spacing

- [x] Read the screenshot and trace exact live rows using read-only access.
- [x] Trace typed authorship and originating tool calls; never infer it from prose.
- [x] Keep internal coordination out of bot chat, including existing history, while preserving human messages and intentional bot deliveries.
- [x] Preserve internal thread/Activity observability.
- [x] Trace exact spacing through native source, tool input, persistence and rendering; fix any Jaz loss at its owner.
- [x] Add meaningful normal-path regressions and verify production rendering hidden.
- [x] Apply the thermo-nuclear code quality review and fix findings.
- [x] Report cause, fix and evidence before local merge.

Live SQLite reads use `mode=ro`. No live history writes, provider prompts, or agent launches are needed for this projection repair.

Cause, fix, exact provenance, whitespace findings, verification and review: [review-bot-message-attribution.md](review-bot-message-attribution.md). Local merge remains pending the pre-merge report.
