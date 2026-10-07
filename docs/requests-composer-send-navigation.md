# Sent Message Reappearing in Composer

- [x] Reproduce sending in an existing chat, navigating away before acknowledgement, and returning to a populated composer.
- [x] Preserve message submission across navigation and detach live observation after acknowledgement.
- [x] Verify sent drafts stay cleared, rejected sends remain retryable, unsent drafts survive navigation, and Stop still cancels immediately.
- [x] Run full checks and strict code review.
- [x] Commit the fix.

Follow-up strict review:

- [x] Reproduce a late rejection overwriting a new draft after returning before acknowledgement.
- [x] Invalidate rollback on later persisted draft changes across composer mounts; restore a rejected draft into an already reopened composer.
- [x] Scope the HTTP fixture and verify the additional timing cases, full checks and final review.
- [x] Commit the reviewed correction.

Cause: navigation aborted the POST/SSE request before its durable `accepted` event arrived. The composer treated the abort as a send failure and restored the saved draft, even when the server had persisted the message.

The send hook owns the controller and acceptance promise together. Navigation waits for acceptance to settle, then aborts live observation; explicit Stop still aborts immediately.

Verification: the browser fixture failed against the original hook with an accepted message restored into the composer. The corrected hook passes the same fixture, including delayed HTTP headers, delayed SSE acknowledgement, unsent and rejected drafts, retry, a new draft and Stop. The fixture runs in the standard browser smoke suite and independently with `bun run test:browser --composer`. It was executed through Jaz's side browser using the real React composer, hooks and stream parser, with only HTTP responses controlled.

Follow-up review found a rollback defect missed by the initial checks: return before rejection, write a new draft, then receive the rejection. The old composer's refs still described an empty draft, so rollback overwrote the newer saved draft. That sequence failed against `f49c02c7`.

Draft persistence now publishes changes per storage scope and key. A pending rollback subscribes after clearing and becomes invalid when any later text, attachment or context mutation occurs. The subscription ends when the send settles. Text editors subscribe to the same changes, so a rejection can restore the draft into an already reopened editor. Existing component checks continue to protect ephemeral fields and goal state.

The browser check now covers late rejection after remount with new text, a new file or a quote; rejection after an empty return; and draft isolation between chats. The HTTP fixture intercepts only its two test-chat stream endpoints. All cases pass. Final review found no remaining blockers; full backend tests, 307 frontend tests, typecheck, lint and web/desktop bundle builds passed.
