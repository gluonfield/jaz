# Engineering Principles

- Use modern Go 1.26. Generics are available and should be used when they make code smaller or clearer.
- Write as little code as possible. Prefer concise, obvious implementations; every new line must justify itself.
- Every JSON field must justify itself too. The default is to omit fields and code until there is a concrete reason to keep them.
- Keep structure clean as the project grows. Put concrete implementations in focused packages and keep core interfaces small.
- Prefer Viper's default field-name mapping for config structs. Do not add `mapstructure` tags unless there is a specific mismatch that cannot be solved cleanly by naming.
- When designing agent/runtime behavior, clone and inspect these reference projects for inspiration:
  - `openclaw`: https://github.com/openclaw/openclaw
  - `hermes`: https://github.com/nousresearch/hermes-agent
- Treat reference repos as learning material, not authority. Some implementations will be good, some will be poor, but reading them should still sharpen our design choices.
- Tests must earn their place. Add tests only when they protect meaningful behavior, catch likely regressions, or clarify a tricky contract.
- Avoid coverage theater. A test that is not genuinely useful should not live in this codebase.
