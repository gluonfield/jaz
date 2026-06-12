# Engineering Rules

- Use Go 1.26.
- Keep code and JSON minimal. Each line of code should fight for its existence; every field and line must earn its place.
- Do not write code comments unless they explain something the code itself cannot describe.
- Keep concrete implementations focused and interfaces small.
- Put behavior in the layer that owns the concept. Shared transcript/message shapes belong in storage or a dedicated shared package, not copied through server, ACP, and UI paths.
- Keep provider-facing data separate from display/transcript data. Do not mutate prompts and then repair snapshots by string matching; carry explicit typed boundaries instead.
- Architect the native Jaz agent behind protocol-shaped interfaces, modeled after MCP-style request/response/event contracts. Typed content blocks, tool calls/results, permissions, streaming updates, and capabilities should cross explicit interfaces instead of direct server/provider coupling.
- Keep native runtime behavior transport-neutral. Native, ACP, and future protocol adapters should share internal turn/session/tool contracts; protocol-specific code translates only at the boundary.
- Preserve migration paths to ACP or MCP-style protocols when adding agent features. Prefer capabilities and feature detection over hardcoded runtime branches, and do not hide native-only semantics inside prompts.
- Split files when a feature starts mixing transport, persistence, formatting, and UI concerns. Avoid pushing files toward 1k lines without a strong structural reason.
- Frontend shared hooks and lib code must not import component-owned types. Put cross-layer contracts in `lib`.
- Keep feature diffs scoped. Do not mix unrelated UI polish, settings work, dependency churn, or generated output into behavioral changes.
- Prefer Viper's default field mapping. Add `mapstructure` tags only for real mismatches.
- Use Fx constructors directly in `fx.Provide`; avoid pass-through wrappers.
- Codex ACP uses the user's Codex OAuth credentials. Never pass coordinator provider keys to Codex subprocesses.
- Target deployments run the Jaz server on a VM and clients on user computers; never assume client-local file paths are visible to the server or agents.
- Every test you add must be useful: it must protect real behavior or clarify a tricky contract, never exist only to raise coverage.
- Reference repos (`openclaw`, `hermes`) are learning material, not authority.
