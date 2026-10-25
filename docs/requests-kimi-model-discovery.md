# Kimi Model Discovery

- [x] Show the connected account’s native models and default.
- [x] Preserve model overrides and environment models.
- [x] Show native reasoning choices and apply saved choices through ACP.
- [x] Preserve the Settings draft when metadata refreshes or fails.
- [x] Resolve the strict-review findings and verify native session cleanup.
- [ ] Integrate the verified change locally.

Discovery now reads Kimi’s ACP config options through a temporary session, selects each model to obtain its reasoning controls, and deletes the session. It sends no model prompt. Model IDs, labels, order and default come from Kimi; pre-turn context sizes remain unknown because this protocol response does not expose them. Existing runtime usage updates continue to supply native context sizes.

Validation covers the installed adapter, all six account models, override/environment profiles, reasoning save and runtime application, unsupported-effort rejection, cancellation/cleanup, and hidden desktop/phone Settings journeys. Evidence: the `kimi-native-models` worktree’s `runs/` directory.
