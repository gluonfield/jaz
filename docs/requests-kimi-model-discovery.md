# Kimi Model Discovery

- [x] Investigate connected Kimi with no model choices (25 October 2026 screenshot).
- [x] Read the selected native profile for model aliases, labels, context sizes and default.
- [x] Verify discovery, native parity and the actual Settings picker.
- [x] Complete strict review and integrate locally.
- [ ] Resolve the follow-up native-parity findings before release.

The account profile contains six models. Kimi previously fell through to the static catalogue, which has no Kimi entries. Discovery now uses the same resolved native profile as authentication and execution. Missing context metadata stays unknown. The follow-up review withdrew the initial sign-off: the installed adapter supports granular reasoning levels, and its effective catalogue can differ from the stored TOML fields.

Validation: Go 1.26.3 ACP/model-capabilities suites and vet; the regression fails against the old discovery code. Six stored profile rows matched the native provider-list output. Hidden Settings checks cover desktop/phone, save, refresh, reconnect and empty/error responses. That evidence does not establish effective native-catalogue parity; see [follow-up review](review-kimi-model-discovery.md). The change remains unreleased.
