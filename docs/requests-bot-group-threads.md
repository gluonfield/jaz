# Bot Group Threads

- [x] Hand a group's new posts to a member while it works on its turn, instead of dropping the ones that arrive mid-turn.
- [x] Leave a member's pending question to the user when group posts arrive.
- [x] Merge the durable bot delivery and restart recovery work (0097c081).
- [x] Run each bot's group turns in a thread of its own per group, so group work neither blocks nor reroutes the bot's own chat and nothing lives only in memory.
- [x] Pick up after a restart: resume interrupted bot, worker and group threads, and keep queued group turns.

Measured on the installed Claude adapter: Jaz's prompt queueing already delivers a mid-turn message at the next tool boundary, while `_session/steering` interrupted the running command and the model dropped the message, so Claude stays on prompt queueing.

A bot's group thread is a hidden `bot_member` thread on the bot's agent, model and home, created before its first turn in the group and recorded in `bot_memberships` with the seq of the last group message shown to it. Its identity prompt says it speaks to the group; `send_message` there posts to the group as the bot, and a reply turn still answers the bot that asked. Model and agent changes reach a bot's group threads; deleting a bot or group archives them. Routines created from a group thread belong to the bot, and its usage counts as the bot's.

Restart: startup recovery resumed only the first of a worker and its parent, because a resumed worker draws its parent's attention and the recovery loop skipped sessions whose update time had changed. The status check already rejects sessions that moved on, so that guard is gone.

Verification: full Go tests, bot tests under race, 306 frontend tests, typecheck and lint pass. Negative controls for each behaviour (turns in the main chat, unsaved read position, waking on unaddressed posts, unsynced models, unmapped routine owner, apps shown from a group thread, missing group identity, missing bot tools, the restart guard) fail on the expected test.

## Requested Strict Review

- [x] Audit the group-thread change for structure, durability and duplicated mechanisms.
- [x] Fix confirmed findings, verify and commit.

The review replaced the in-memory turn scheduler (taking/owed flags, a wake loop and a startup pass that rebuilt lost wakes) with the server's durable per-thread queue, which already runs queued turns when a thread is free and survives restarts. A post now goes to every member it addresses and every member whose group thread is taking a turn as it is posted: delivery steers it into that turn, or queues a turn on the thread when it takes none or cannot take the messages, and only then marks them seen. Deciding at post time closed a gap where a message posted in a turn's last moments was judged after the turn ended and never delivered. The follow-up cap is spent only when something is delivered. Net change: 367 lines removed, 187 added.

Verification: full Go tests, bot tests 50 times plain and 10 times under race, and controls for turns in the main chat, mid-turn steering, the post-time turn check, a refused queue, the follow-up cap, unsynced models, apps shown from a group thread and missing group identity all fail on the expected test. The first-turn fallback to a bot's own last post is no longer reachable by any test, because deliveries now keep the read position current; it remains for groups that predate group threads.

## No One Working After A Group Post

- [x] Find why a post mentioning CEO in "Business Dev Team" started no turn.
- [x] Fix it and make delivery failures visible in the group.

CEO's group thread was never created: thread creation copied the bot's stored model provider, which for Claude is the agent's own name, and the effort check then looked for a provider catalog named "claude" ("unknown model provider"). Bot subtasks copied it the same way, which is why no bot worker has ever been created. The agent manager now creates a bot's subtasks and group threads alike from the bot, passing its provider only when it is a real one, and the group shows "Couldn't reach CEO · …" when a delivery fails instead of failing silently.

Verification: a real-manager regression with a Claude bot creates both threads and fails with the old copy on the same error; full Go and 307 frontend tests, typecheck and lint pass; bot tests pass 40 times plain and 10 under race; removing the notice fails its test.

## Files In Group Chats

- [x] Accept dragged and pasted files in a group chat, as bot chats do.
- [x] Strict review: resolve the files against the loaded group's thread.

A group now uses the bot chat composer inside a file drop scope. Files upload to the group's thread, travel on the room message (server paths hidden from clients, as with side chats) and appear under the message in each member's group turn with their server path. A post can be files alone. Review: the bots handler resolved ids against the raw `{bot}` path value before the group was loaded, and Go's router delivers `..` and `a/../b` from encoded segments; `Post` now loads the group first.

Verification: full Go and 310 frontend tests, typecheck and lint pass; removing the prompt line, the path stripping or resolving against another thread each fails its test. An isolated backend with a fake agent showed the drop overlay, the uploaded chips, both posts and the members' queued turns naming each file's server path.

## Every Post Reaches Every Member

- [x] Find why CEO stopped answering in "Business Research".
- [x] Hand every group post to every other member and let each decide whether to answer.

The Researcher (Codex) addressed CEO as plain-text `@CEO` after CEO's turn had ended. Only `[@Name]` and `bot:` links counted as mentions, and a member's post with none woke nobody, so the post was stored but never delivered. Mention-only waking was introduced on 30 September to cut silent reaction turns; the user's rule is that no member misses a post. Every post now goes to every other member; mentions only link a name to its bot. The follow-up cap still bounds bots answering each other between the user's posts, holding the excess as unseen until the next user post.

Verification: the plain-tag regression fails under the old rule; bot tests pass 40 times plain and 10 under race; full Go tests pass.
