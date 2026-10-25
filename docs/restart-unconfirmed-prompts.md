# Recovering Unconfirmed Prompts

A restarted turn explicitly marked sending ends with an interrupted
result when Jaz has no durable confirmation. It receives neither its original
prompt nor a continuation. Its error is:

> Jaz restarted before the agent confirmed it had this task

The parent receives that result through the existing atomic turn-completion path.
Its commitment stays open. The parent or commitment watchdog chooses the next
step. User threads retain the original visible prompt and show the error.

An interrupted continuation with an explicitly acknowledged origin instead reports:

> Jaz restarted before the agent confirmed the continuation. Earlier task progress is saved.

The active turn's optional `ContinuationOf` retains the original acknowledged
start time across prepared and confirmed recovery attempts. Each attempt keeps a
fresh `StartedAt` for confirmation-write guards. Legacy evidence stays unknown;
compaction, fresh instructions and new queue claims acquire no inherited origin.
Finalization selects the reason before clearing the active turn and ignores a
duplicate failure for an already-terminal session. This preserves diagnosis;
automatic recovery for ambiguous continuations remains unresolved.

| Durable prompt state | Recovery |
| --- | --- |
| Prepared, with no transport attempt | Send the original input once |
| Sending, without confirmation | End as interrupted |
| Confirmed by primary turn output or a prompt response | Continue once through the existing path |
| No prompt record: older written turn or agent-started run | Continue once through the existing path |

Jaz records sending before writing to the transport, covering a crash between a
pipe write and its callback. It confirms at the first primary message, thought,
plan or tool update, or any response to that turn's `session/prompt`, including
cancelled, error and typed provider-failure responses. After one successful
confirmation write, later observations do not write again. A failed write permits
another attempt only on a later qualifying observation. Metadata,
side-chat output and load-time history replay do not confirm a task. A failed
confirmation write leaves recovery uncertain. State writes carry the turn's
start timestamp, so a stale state write cannot confirm a later task.

The parent result uses state `interrupted`. Persistence uses the existing terminal
`error` status: the stored `interrupted` status means recoverable work and would
otherwise retry the turn or block the watchdog. No new persisted lifecycle or
provider contract is introduced. Existing compaction-command recovery is unchanged.

| Turn kind | Prompt record and restart behavior |
| --- | --- |
| Chat, worker, bot result, routine or planning prompt | Prepared input delivers once; confirmed input continues; explicitly sending and unconfirmed input interrupts |
| Claimed queued prompt or action | Existing server recovery starts its claimed prompt or action |
| HTTP turn reservation before ACP prepares its input | No prompt record yet; keeps the existing continue-once recovery |
| Claude background follow-up, claimed by the agent's work report | No client prompt; already running by definition; continues once and retains recipients |
| Goal turn, including automatic goal continuation | Normal prompt states; goal intent survives continuation |
| Provider retry continuation | Same turn and confirmation; response-only failures confirm before the retry wait; used retries survive restart |
| Native steer joined to a turn | Keeps the owning turn's state, intent and visible messages; confirmed owner continues once |
| Compaction | Existing operation recovery sends `/compact`, including older marker-less records |
| Older written turn with no prompt record | Existing continue-once recovery; the upgrade does not stop these turns |

Verification uses the fake ACP agent through production transport, recovery,
completion and SQLite. It covers worker, user and hidden bot-result turns, native
actions before the first update, confirmation commit boundaries and old records.
Controls remove each critical safeguard and must fail at the intended assertion.
No live provider runs are part of this change.
