# Claude Compaction

- [x] Use a 400,000-token compaction window for Claude.
- [x] Put the default in Jaz's built-in Claude configuration so it ships to all users.
- [x] Use Claude Code's native environment setting through the existing launch path.
- [x] Run the backend checks, complete the thermo-nuclear review and commit.

The environment default overrides Claude's saved compaction window. Jaz's agent configuration can override or clear it. Existing Claude processes need a relaunch after updating. The local profile override remains until the updated build is running.

Validation: Go 1.26 full backend suite, `go vet ./...`, 361 frontend tests and thermo-nuclear review passed. A live 400k compaction has not been exercised.
