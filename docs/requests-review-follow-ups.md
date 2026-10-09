# Review Follow-ups

- [x] Make file links flow inline like web links.
- [x] Find and fix the flaky bot chat history smoke stage.
- [x] Decide on Plan and Goal chips in bot chats.
- [x] Run full checks and the full browser smoke suite; commit.

File links were `<button>`s, and a button can't flow inline, so text after a wrapped file link started a new line. They are now inline `role="link"` spans that open on click or Enter and share `.chat-prose-link`. The button reset CSS and the icon's `flex-shrink` are gone.

The flaky stage exposed a real paging bug. Chromium skips native scroll anchoring at scroll offset 0 (measured in Jaz's Electron: adding 500px above moved offset 1 to 501 and 40 to 540, but left 0 at 0). A reader who reached the very top of a bot chat or thread jumped to the oldest revealed entry. The sentinel stayed in view, so the window kept revealing batches and then fetched earlier pages. `useHistoryWindow` now applies its manual anchoring at offset 0 and leaves native anchoring elsewhere, which still handles image reflow. The stage now asserts that the reader stays on Reply 90: 10/10 pass with the fix and 10/10 fail without it, where the timing check had failed about 1 in 20.

Plan and Goal chips stay in bot chats. `planActive` is the provider's own current mode, so a Claude bot can enter plan mode by itself, and the chip is how a menu-less bot chat shows and exits it. Removing it would hide provider-owned state.

Verification: typecheck, lint, 311 frontend tests, two full browser smoke runs and four composer suite runs pass. A fixture showed file links wrapping across two lines with the following text on the last line, taking focus and opening on click and Enter.
