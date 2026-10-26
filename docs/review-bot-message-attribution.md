# Bot Coordination Visibility and Whitespace

Task126, 26 October 2026. Pre-merge review of `jaz/bot-message-attribution`, approved by the coordinator at `b10e28a9`. Live inspection used SQLite `mode=ro`; production data was never edited.

## Cause and Exact Provenance

The screenshot's purple bubbles are CAS soft thread `20261018T165847-24ebef56`, message records 285–287. All have `role=user`, `from_user=0`. The protocol role identifies agent input; the persisted authorship flag identifies human input. Both the SQL chat projection and frontend chat projection used role alone. Paginated hydration also rebuilt message records without carrying `from_user`.

All three originated in worker `20261024T063105-9d99a3b0` through `send_message_to_thread`, targeting the CAS soft thread:

| Message | UTC | Source tool event | Native rollout line | UTF-8 bytes | SHA-256 prefix |
|---|---|---|---|---:|---|
| 285 | 08:49:22.938 | 2358 | 3680 | 527 | f9dd873f30a1 |
| 286 | 08:52:49.485 | 2393 | 3735 | 469 | 06ca79ba1f99 |
| 287 | 08:56:28.343 | 2415 | 3778 | 548 | 5dda28bca730 |

The native source is `/Users/wins/.jaz/acp/codex-home/sessions/2026/10/24/rollout-2026-10-24T07-31-05-01a16c9b-17de-7e91-8f5b-4a61d203e701.jsonl`. Exact tool IDs, native call IDs, complete hashes and equality checks are in `runs/bot-message-attribution/source-trace.json`.

## Fix

- Chat SQL filters and pagination boundaries use persisted human authorship. Internal rows remain in the full transcript; existing history is corrected when projected, without a migration or deletion.
- Hydration reuses the canonical database decoder, removing its duplicate and preserving authorship through the HTTP response.
- Bot chat accepts typed human messages and intentional room deliveries. Internal sent/received activity rows remain available in Activity rather than chat.
- Optimistic human sends carry their origin; an internal arrival cannot acknowledge, replace or dismiss them.
- Bot chat pagination no longer uses assistant-row alignment: bot replies live in event records. This preserves earlier human messages among frequent coordination updates.

## Whitespace

For each exact screenshot record, native tool-call text, MCP input, stored `content` and stored text block are identical. The production markdown DOM also reproduces these strings exactly. The compressed words, including `acknowledgesnewcriterion` and `throughACTUALhostedFirebase`, already occur in the native source. No Jaz whitespace loss was found on this path; the source text remains intact.

Spacing regressions cover MCP steering, SQLite persistence/reopen, history projection and production rendering. Repeated spaces, tabs and line breaks remain in recorded text and DOM; browser layout applies normal Markdown whitespace presentation.

## Verification and Review

- The new SQLite paging regression and frontend authorship regression failed on the original code and pass with the fix. HTTP coverage preserves human and internal records with identical prose, while bot chat returns only the human record.
- Go 1.26.3: storage, SQLite, JSON store, server, transcript, bots, ACP and sessions HTTP suites pass; affected-package vet passes.
- Frontend: all 404 tests, typecheck, ESLint and desktop bundle pass. The final focused assertions also pass.
- Hidden production `BotChat`/`ChatLog` checks cover live and reloaded history, desktop/phone layouts, human row/event inputs, attachments, deliberate replies and internal coordination exclusion. The fixture is inside the captured viewport and screenshots were inspected.
- Exact live-record replay retains human message 283 and deliberate room deliveries 13715, 13718 and 13723; internal records 284–287 are absent. Evidence: `runs/bot-message-attribution/replay/cas-soft-history-{desktop,mobile}.png` and `exact-replay.log`.
- Thermo-nuclear self-review: no remaining actionable findings. The repair reuses the existing typed contract and canonical decoder, with no new origin schema, provider mutation, prose classifier or historical rewrite. All changed implementation files remain below 1,000 lines. Review also caught and fixed a blank screenshot fixture before accepting visual evidence.

Provider execution, metadata, authentication, prompts, context windows and compaction are untouched. Native model runs were unnecessary for this display/storage projection change; existing MCP steering and routing regressions pass. A focused smoke entrypoint isolates bot history from the larger browser suite. No dependency versions changed.

Logs and private replay inputs are under ignored `runs/bot-message-attribution/`. No push, release, restart or live history write was performed.

## Final Integration

Current main was integrated with one selector-list conflict, resolved by retaining both `claude-voice` and `chat-history`. The inline-link change `5522d632` and later native Claude voice defaults `791df21f` are preserved. Every reviewed bot feature and regression-test blob remains identical to `b10e28a9`; only the smoke runner combines the two selectors. The prior 404-test frontend evidence is reused.

The full Go 1.26.3 suite ran once. It exposed two stale voice-client tests, whose scripted boundary lacked the newly required voice-settings response, and a skill-install test that assumed umask-independent permissions. Test-only corrections supply the settings contract and compare installed modes against native file creation under the inherited umask, without changing production behavior or the process umask. All failing voice scenarios, the skills package and the later voice-default package pass focused reruns. Full vet passed; touched-package vet and final hidden rendering are recorded with the integration receipt. The broad suite was not repeated.

Combined desktop/phone bot-history rendering passed after the inline-link integration. The final verification and local `--no-ff` merge/tree-equality receipt live under `runs/bot-message-attribution/integration/`. The single full-run failure log remains available alongside the successful focused corrections. No push, release or restart is part of this integration.
