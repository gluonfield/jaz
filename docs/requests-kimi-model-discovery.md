# Kimi Model Discovery

- [x] Investigate connected Kimi with no model choices (25 October 2026 screenshot).
- [x] Read the selected native profile for model aliases, labels, context sizes and default.
- [x] Verify discovery, native parity and the actual Settings picker.
- [x] Complete strict review and integrate locally.

The account profile contains six models. Kimi previously fell through to the static catalogue, which has no Kimi entries. Discovery now uses the same resolved native profile as authentication and execution. Missing context metadata stays unknown. Kimi's ACP interface exposes thinking as an on/off control, so this change does not advertise the profile's granular efforts as selectable ACP efforts.

Validation: Go 1.26.3 ACP/model-capabilities suites and vet; the regression fails against the old discovery code. The real HTTP handler matches all six models from the installed native CLI. Hidden production Settings checks cover desktop/phone, save, refresh, reconnect, empty/error responses and the native default. Evidence and strict review: worktree `runs/`. The installed app needs a release to receive this change.
