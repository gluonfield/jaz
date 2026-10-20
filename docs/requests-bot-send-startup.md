# Bot Message Startup Failure

- [x] Diagnose the failed bot send from the screenshot and macOS crash reports.
- [x] Repair the installed Claude adapter and verify a real ACP initialize response.
- [x] Confirm the bot starts after the user's app restart.
- [x] Correct and verify standalone macOS release packaging.
- [x] Review, commit the packaging correction and record the result.
- [x] Merge the reviewed fixes into main and push (authorized after review).

The Claude adapter's invalid Mach-O code signature caused macOS to kill it
before ACP initialization. The installed adapter was re-signed without changing
the agent implementation or Claude runtime. The original executable is retained
in the ignored `runs/bot-send-signature/` evidence directory.

Packaging correction: Claude fork commits `29f4ffc` and `f090c98` on
`jaz/standalone-macos-signature`. Builds run on macOS, sign the compiled
adapter, verify the adapter and vendor runtime, and smoke the native
executables before archiving. The focused packaging patch is pushed to the
Claude adapter's main branch as `0245630`; its existing agent source and
runtime pins are preserved. The release-based review branch is also pushed.
Corrected archives are published as Claude adapter `0.93.0-jaz.3`, built from
`0.93.0-jaz.2` plus the reviewed packaging fixes. Jaz pins the new release.

Verification: published and installed failing binaries have identical hashes;
the repair preserves the compiled `__BUN` payload. The original signature
fails verification; the repaired package passes verification before and after
archive extraction, both version commands, and a credential-free real ACP
initialize. The bot saved a reply and completed its turn after the user's
restart. The strict maintainability review, adapter lint/format/build, 3,131
active adapter tests, and full Go 1.26 backend suite passed. The actual local
five-platform packaging run also passed: every archive contains only its two
executables, OS/CPU headers are correct, and both macOS pairs retain valid
signatures after extraction. The GitHub release workflow was not dispatched.

Main integration passed its 1,229 active adapter tests, lint/format/build,
five-platform packaging, extracted macOS signatures and credential-free
native ACP initialize. Jaz's full 103-package backend suite and vet passed.
