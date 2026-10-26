# Inline File Link Layout

- [x] Reproduce the reported `its code` link through production `UserMessageMarkdown` and its stylesheet.
- [x] Correct icon alignment and wrapping with scoped CSS.
- [x] Verify hidden before/after captures at narrow/wide widths in light/dark.
- [x] Complete focused checks and the required thermo-nuclear review.
- [x] Commit the verified fix and provide local integration evidence.

Source: Augustinas's screenshot and request, 26 October 2026.

Eleven CSS lines anchor the file icon in the inline link's leading padding. This keeps it with the first text fragment while labels wrap naturally. Scope: `.chat-prose-link[role="link"]` and its icon.

Hidden Electron captured production `UserMessageMarkdown`, `FileReaderLinkProvider` and the compiled stylesheet at 360/1040 px in light/dark (eight before/after images). Short, descriptive, unbroken-path, code and bold labels were inspected. A 120–360 px sweep finds 16 detached-icon widths before and zero after; normal-text icon centre offset improves from 2.58 px to 0.04 px. No horizontal overflow; click/Enter handlers receive the file reference. Every capture window remained invisible and unfocused.

Focused verification: 24 Markdown/file-reference tests, frontend typecheck, Vite production-component build and whitespace check pass. Thermo-nuclear self-review: no blockers; layout stays in CSS without new parsing, state, wrappers, dependencies or provider changes. Evidence and reproducible capture scripts: `runs/inline-file-links/` (gitignored).
