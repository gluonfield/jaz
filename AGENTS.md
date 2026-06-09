# Engineering Rules

- Use Go 1.26.
- Keep code and JSON minimal; every field and line must earn its place.
- Keep concrete implementations focused and interfaces small.
- Prefer Viper's default field mapping. Add `mapstructure` tags only for real mismatches.
- Use Fx constructors directly in `fx.Provide`; avoid pass-through wrappers.
- Codex ACP uses the user's Codex OAuth credentials. Never pass coordinator provider keys to Codex subprocesses.
- Target deployments run the Jaz server on a VM and clients on user computers; never assume client-local file paths are visible to the server or agents.
- Add tests only when they protect real behavior or clarify a tricky contract.
- Reference repos (`openclaw`, `hermes`) are learning material, not authority.
