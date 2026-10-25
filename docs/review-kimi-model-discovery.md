# Kimi Model Discovery Review

**Result: changes required.** Review of `404a3603` / integrated `01f4794e` against thermo-nuclear-code-quality-review. The initial self-review's approval is withdrawn.

1. **P1 — Stored configuration is used as the native catalogue.** `kimi_models.go:11–18` bypasses Kimi's effective model registry. An isolated profile with an alias override produces “Effective label” through the installed native ACP adapter and “Stored label” through Jaz. With the same `KIMI_MODEL_*` environment in both processes, native ACP offers two models and selects its environment model; Jaz offers one and reports the stored default. Overridden context sizes are also ignored. Discovery must consume the provider-owned effective catalogue and default, including its declared capabilities, rather than grow a parallel native-config resolver.

2. **P2 — Supported reasoning controls are reported as unavailable.** `kimi_models.go:19` unconditionally disables reasoning capabilities. The installed adapter's real `session/new` response advertises Low, High and Max for the audit model. This hides the Settings control and causes an existing effort to be removed from the editable Settings draft. The earlier statement that this adapter supports only an on/off toggle came from inspecting another source revision. Correct discovery must preserve the native choices and verify selection/save/runtime application through the existing session config contract.

The production additions are small, use the existing profile resolver and introduce no dependency or oversized file. Those structural strengths do not resolve the incorrect metadata boundary. The regression test currently asserts `unavailable`, encoding the second defect; it needs native-contract coverage.

## Evidence

- Installed Kimi `2.1.1-jaz.1`, SHA-256 `e2ee7c10e1ece7e79b30ccd390ab64a155bb62b73a7ea7c1111924fec930ac2a`.
- Exact source tag `v2.1.1-jaz.1`: `packages/acp-server/src/model-catalog.ts`, `config-options.ts`, and `packages/node-sdk/src/config/{model,env-model,toml}.ts`.
- Actual installed ACP `initialize` / `session/new` compared with production `Manager.DiscoverModels`, once with an override and once with an environment model. Synthetic profile and placeholder API key; outbound network denied, writes confined to review evidence. No model prompts were sent.
- Raw responses and asserted comparison: `/Users/wins/.jaz/workspaces/default/.worktrees/kimi-model-discovery-review/runs/` (`native-handshake.json`, `native-env-response.json`, `jaz-response.json`, `jaz-env-response.json`, `comparison.json`).

This follow-up changes documentation only. Both implementation findings remain open; the installed app and its settings were not changed.
