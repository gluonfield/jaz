# Bot Message Startup Failure

- [x] Diagnose the failed bot send from the screenshot and macOS crash reports.
- [x] Repair the installed Claude adapter and verify a real ACP initialize response.
- [x] Confirm the bot starts after the user's app restart.
- [x] Correct and verify standalone macOS release packaging.
- [x] Review, commit the packaging correction and record the result.

The Claude adapter's invalid Mach-O code signature caused macOS to kill it
before ACP initialization. The installed adapter was re-signed without changing
the agent implementation or Claude runtime. The original executable is retained
in the ignored `runs/bot-send-signature/` evidence directory.

Packaging correction: Claude fork commit `29f4ffc` on
`jaz/standalone-macos-signature`. Builds run on macOS, sign the compiled
adapter, verify the adapter and vendor runtime, and smoke the native
executables before archiving. The correction is committed locally; no new
release was published.

Verification: published and installed failing binaries have identical hashes;
the repair preserves the compiled `__BUN` payload. The original signature
fails verification; the repaired package passes verification before and after
archive extraction, both version commands, and a credential-free real ACP
initialize. The bot saved a reply and completed its turn after the user's
restart. The strict maintainability review, adapter lint/format/build, 3,131
active adapter tests, and full Go 1.26 backend suite passed. The complete
cross-platform release workflow was not dispatched.
