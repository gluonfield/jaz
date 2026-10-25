# Kimi Model Discovery Review

**Result: both findings resolved.** Reviewed against thermo-nuclear-code-quality-review.

- Discovery consumes the installed Kimi adapter’s effective ACP model/config options. The TOML metadata projection and its three duplicate fields are deleted. Actual override and environment profiles now match native labels, choices and default.
- Native reasoning choices are preserved. Unknown native models defer effort validation to native session controls; unsupported values fail before a model prompt. Clearing an inherited effort reaches the existing no-override path.
- Settings normalizes an effort when the person changes models. The reactive effect that silently cleared saved choices during catalogue refresh is deleted.

The lifecycle uses the existing ACP connection implementation and requires native session-deletion support before creating its temporary session. Success, model-selection failure and cancellation delete that session; deletion failures surface as errors. The installed adapter leaves zero sessions in all three discovery workspace scopes. No new dependency, provider patch, static model/effort list, native prompt, tool or authentication change was introduced.

Verification: Go 1.26.3 backend packages and vet; Settings save/runtime wire-contract regressions; 376 frontend tests, typecheck and lint; actual installed `2.1.1-jaz.1` metadata for the account and two controlled profiles; hidden desktop/390px save, refresh, reconnect and error-state journeys. The old Settings effect fails the same empty-catalogue journey that the corrected component passes. Backend fixtures under the project use a canonical temporary path, a Git discovery ceiling and umask 022.

The original failing native/production comparison is retained in `kimi-model-discovery-review/runs/`; corrected evidence and the final integration receipt live in `kimi-native-models/runs/`. No model inference or live Settings write was used for verification.
