# Connect cards in chat

- [x] Propose how agents can offer connections as cards, modelled on Meta's connector links.
- [x] Drop the separate agent tool; no inline pills, since providers can hold several accounts.
- [x] List `jaz://connect/<plugin>` in the prompt only for unconnected Connections catalog providers.
- [x] Render a link alone on its own line as a live card in assistant and bot chats; code and sentences keep the text.
- [x] Reuse the Settings connect flow; connected multi-account providers offer Add account. User-added MCP servers stay out.
- [x] Verify rendering, run full frontend/backend checks and strict review; commit on the branch.

The prompt status comes from the same plugin status the Settings page shows. The card resolves its id against the catalog, so unknown ids stay plain text. 312 frontend tests, typecheck, lint, and the full Go vet/test suite pass. Light and dark rendering was checked from the real components; the side browser was unavailable, so the click-through sign-in was not exercised live. Unverified: whether an agent already running in a thread sees tools for a provider connected mid-thread.
