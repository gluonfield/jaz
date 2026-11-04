# Delegation Message Spacing

Request: fix unreadable word spacing in delegated thread messages, shown by CAS P5's "FINAL VERIFIED PRODUCER HANDOFF" for `8e25e419`.

- [x] Trace the example's authored, tool, stored and rendered bytes.
- [x] Check whether the prior attribution fix (`834402de`) is included and whether it covered this cause.
- [x] Correct the proven cause at the Jaz-owned boundary, preserving identifiers.
- [x] Run focused checks and a thermo-nuclear self-review.

Evidence: the native Codex rollout records P5 composing the message as a JavaScript string literal inside its `exec` code tool. The model-authored literal, both `send_message_to_thread` tool inputs, the stored `content` and text block in both receiving threads, and the production `UserBubble` text are identical: 1,175 ASCII bytes, SHA-256 `2031eebc…`. Jaz did not remove any spaces. `834402de` is an ancestor of `main`; its whitespace finding still holds, and it made no policy change.

Across 342 Codex rollouts from 3–4 November, 392 of 1,819 thread messages written in `exec` literals have heavy word gluing, against 0 of 5,868 assistant replies from the same sessions. In P5, gluing rises with context: a mean of 0.02 of words below 100k tokens, 0.12 above 200k. It falls after each compaction. Codex bots show the same split. Six seconds before the handoff, P5's own reply stated the same facts with normal spacing.

Change: the thread and bot prompts now share one sentence: thread prompts are chat messages people read, so write them in plain sentences with normal spaces, including when composing them inside code, and copy IDs, hashes and paths exactly. There is no rejection, rewriting or space insertion at the tool or renderer.

Validation: the existing template, coordinator, app and bots tests pass. Real Codex trials (gpt-6.1-sol, xhigh, code-mode-only, with a stub serving the production tool definitions) kept every identifier exact under both prompts. Fresh and seeded short contexts did not reproduce gluing under either prompt, so these trials show no regression. They do not measure the effect at long context. Receipts are in `runs/delegation-message-spacing/` (gitignored).

Limit: existing work threads keep the prompt they started with. New threads, Claude bot launches and Codex bot identity refreshes receive the sentence. Its effect on long-context drift needs to be measured on new sessions after release.
