# Connect cards in chat

- [x] Propose how agents can offer connections as cards, modelled on Meta's connector links.
- [x] Drop the separate agent tool; no inline pills, since providers can hold several accounts.
- [x] List `jaz://connect/<plugin>` in the prompt only for unconnected Connections catalog providers.
- [x] Render a link alone on its own line as a live card in assistant and bot chats; code and sentences keep the text.
- [x] Reuse the Settings connect flow; connected multi-account providers offer Add account. User-added MCP servers stay out.
- [x] Verify rendering, run full frontend/backend checks and strict review; commit on the branch.
- [x] Thermo-nuclear review: one QR modal props bundle for Settings, onboarding and the card; Add account in the shared action label; skeleton while plugins load; cards in chat renderers only; `Plugin.Available()` for the status check.
- [x] Show the connect-link instruction only when a provider is unconnected; one prompt line per provider.
- [x] Exercise the click in the hidden Electron smoke: card click posts the connect request, opens OAuth externally and shows the new account.

The prompt status comes from the same plugin status the Settings page shows. The card resolves its id against the catalog, so unknown ids stay plain text. 312 frontend tests, typecheck, lint, and the full Go vet/test suite pass. Light and dark rendering was checked from the real components, and the composer smoke covers the click-through (it fails with the click handler removed). Mid-thread connections: Gmail, Calendar, WhatsApp and Telegram tools are always registered, so they work in running threads. Proxied providers (Slack, Drive, Tasks, CRM, Deployink) arrive through `tools/list_changed`; the bundled Claude Code refreshes on it, while Codex's runtime strings show only a log line, so Codex likely needs a new thread. Not confirmed with a live agent turn.
