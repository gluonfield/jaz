# Recovering Unconfirmed Prompts

A restarted turn whose prompt may have reached the agent ends with an interrupted
result when Jaz has no durable confirmation. It receives neither its original
prompt nor a continuation. Its error is:

> Jaz restarted before the agent confirmed it had this task

The parent receives that result through the existing atomic turn-completion path.
Its commitment stays open. The parent or commitment watchdog chooses the next
step. User threads retain the original visible prompt and show the error.

| Durable prompt state | Recovery |
| --- | --- |
| Prepared, with no transport attempt | Send the original input once |
| Sending, without confirmation | End as interrupted |
| Confirmed by primary turn output | Continue once through the existing path |
| Older record without a confirmation marker | End conservatively as interrupted |

Jaz records sending before writing to the transport, covering a crash between a
pipe write and its callback. It confirms at the first primary message, thought,
plan or tool update, with one confirmation write attempt per turn. Metadata,
side-chat output and load-time history replay do not confirm a task. A failed
confirmation write leaves recovery uncertain. State writes carry the turn's
start timestamp, so a stale state write cannot confirm a later task.

The parent result uses state `interrupted`. Persistence uses the existing terminal
`error` status: the stored `interrupted` status means recoverable work and would
otherwise retry the turn or block the watchdog. No new persisted lifecycle or
provider contract is introduced. Existing compaction-command recovery is unchanged.

Verification uses the fake ACP agent through production transport, recovery,
completion and SQLite. It covers worker, user and hidden bot-result turns, native
actions before the first update, confirmation commit boundaries and old records.
Controls remove each critical safeguard and must fail at the intended assertion.
No live provider runs are part of this change.
