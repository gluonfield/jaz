# Bot Starters Visibility

- [x] Inspect the supplied screenshot and canonical history state.
- [x] Require confirmed empty history, including earlier pages and the persisted event cursor.
- [x] Verify loading, pagination, reopening, bot-only history, fresh onboarding and real unanswered questions in hidden production components.
- [x] Run focused checks and the requested thermo-nuclear review; resolve findings.
- [x] Commit and integrate locally, verifying the tested and merged trees match.

`BotChat` previously inferred onboarding from the absence of a projected user entry. Established bot-only history therefore showed starters. `ThreadView` now supplies its empty state, qualified by no earlier pages and a zero persisted event cursor; `BotChat` consumes that state directly. Initial loading already prevents the chat renderer from mounting. No persistence or question handling changed.

The normal welcome smoke now also renders `ThreadView` through `BotChat` against controlled HTTP history responses. It exercises unresolved initial loading, bot-only delivery and reopening, delayed earlier-page loading, history whose rows are filtered out, assistant-only raw messages, an actionable unanswered question, and empty-history reopening. The existing welcome checks retain new-bot typing and first-message starter actions.

Verification: frontend typecheck, focused ESLint, 20 bot/history tests, desktop bundle, and hidden Electron welcome smoke using Go 1.26.3. Desktop/mobile captures were inspected. Evidence: `runs/bot-starter-visibility/`; screenshots: `runs/browser-smoke-O1GJvE/`.

Thermo-nuclear self-review: the fix replaces a projection heuristic with state derived by the history owner. One required view field carries it; no helper, storage flag, dependency or duplicated policy was introduced. The normal empty-transcript UI and real question rendering are unchanged. No remaining findings.
