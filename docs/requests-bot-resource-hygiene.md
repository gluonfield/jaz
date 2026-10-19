# Bot Resource Hygiene

- [x] Include archived bot work in managed-worktree pruning, retain active work and preserve source recovery.
- [x] Archive completed workers after a conservative inactivity period following commitment closure.
- [x] Stop eligible idle agents earlier under macOS/Linux memory pressure, oldest first.
- [x] Restore shared worktrees through their recorded owner on every ACP resume path.
- [x] Exercise production behavior, negative controls, isolated subscription-auth checks and a copy-based disk rehearsal.
- [x] Complete the thermo-nuclear review, fix findings, and run the full backend suite.

Verification: full Go 1.26 backend suite, vet, repeated pressure tests, race checks and 14 negative controls passed. Real stop/resume and worktree recovery passed in isolation. Linux ACP tests cross-compile and parser cases pass locally; Linux runtime execution remains unverified. See the [review record](review-bot-resource-hygiene.md).
