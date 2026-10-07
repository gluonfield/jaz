# Sent Message Reappearing in Composer

- [x] Reproduce sending in an existing chat, navigating away before acknowledgement, and returning to a populated composer.
- [x] Preserve message submission across navigation and detach live observation after acknowledgement.
- [x] Verify sent drafts stay cleared, rejected sends remain retryable, unsent drafts survive navigation, and Stop still cancels immediately.
- [x] Run full checks and strict code review.
- [x] Commit the fix.

Cause: navigation aborted the POST/SSE request before its durable `accepted` event arrived. The composer treated the abort as a send failure and restored the saved draft, even when the server had persisted the message.

The send hook owns the controller and acceptance promise together. Navigation waits for acceptance to settle, then aborts live observation; explicit Stop still aborts immediately. Composer persistence and rollback stay unchanged.

Verification: the browser fixture failed against the original hook with an accepted message restored into the composer. The corrected hook passes the same fixture, including delayed HTTP headers, delayed SSE acknowledgement, unsent and rejected drafts, retry, a new draft and Stop. The fixture runs in the standard browser smoke suite and independently with `bun run test:browser --composer`. It was executed through Jaz's side browser using the real React composer, hooks and stream parser, with only HTTP responses controlled.

Strict review found no blockers: the behavior stays in the send lifecycle; no draft flags, transcript matching, provider changes, dependencies or UI changes were added. Full backend tests, 307 frontend tests, typecheck, lint and web/desktop bundle builds passed.
