# Bot Human Message Visibility

- [x] Confirm whether RL Bot received the latest human message.
- [x] Trace the disappearing chat bubbles against persisted records and running code.
- [x] Verify the existing corrected history response and rendered conversation.
- [x] Complete the explicitly requested thermo-nuclear review.
- [ ] Deliver the verified correction to the active service safely.

RL Bot saved the human request at 13:20:28 UTC on 27 October 2026 and intentionally replied at 13:22:35 and 13:24:19. The running backend is the 26 October `791df21f` build, preceding the integrated authorship hydration correction. The current renderer requires typed human authorship, while that server's paginated decoder omits it. Persisted human records retain `from_user=1`; no message replay or history rewrite is needed.

Verification: Go 1.26.3 production backend build; focused HTTP/storage history tests; affected storage/transcript/HTTP vet; hidden production chat checks at desktop and phone widths. The missing-authorship negative control fails the existing HTTP regression for bot, worker and ordinary threads. A read-only production-storage replay preserves authorship on the latest fourteen RL human messages, including the reported request. Screenshots are inspected.

Thermo-nuclear review: the existing correction reuses the canonical row decoder and removes duplicate reconstruction. This incident requires activation of that reviewed correction, with no additional production branches, compatibility guesses, model calls or data rewrites. The active service predates that integration. Three kernel/CAD workers are currently active; restart approval is pending to avoid silently interrupting their work.

Private evidence and the compiled corrected service are in `runs/bot-human-message-visibility/`; visual evidence is in `runs/browser-smoke-xbd906/`.

## Requested Thermo-Nuclear Review

Reviewed the exact authorship repair in `b10e28a9`, its current storage/HTTP/frontend consumers, pagination and optimistic reconciliation, plus the pending activation. No production code-quality blocker or further source change is warranted: the fix deletes duplicate row reconstruction, reuses `messageFromDB`, filters by persisted human origin and retains deliberate room deliveries. SQL page boundaries and returned rows agree; internal coordination cannot acknowledge an optimistic human send. Changed implementation files remain below 1,000 lines, with no added dependency, guessed authorship, provider behavior or historical rewrite.

Additional evidence: 25 focused frontend tests pass. Replaying the original `791df21f` transcript implementation against the live database in read-only mode returns three recent human rows with authorship omitted; the corrected implementation returns fourteen correctly attributed human rows, including message 69. This reproduces the mixed-version failure using the original decoder, supplementing the prior missing-field negative control and hidden rendering checks.

Code review passes. Active delivery remains unfinished: the shared backend is still the older build and three worker threads remain active. The pending restart decision is unchanged by this review. Evidence: `thermal-frontend.log`, `live-original.log`, `live-original-page.json`, `live-corrected.log` and `live-corrected-page.json` under the private evidence directory above.
