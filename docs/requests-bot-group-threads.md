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

The Researcher (Codex) addressed CEO as plain-text `@CEO` after CEO's turn had ended. Only `[@Name]` and `bot:` links counted as mentions, and a member's post with none woke nobody, so the post was stored but never delivered. Mention-only waking was introduced on 30 September to cut silent reaction turns; the user's rule is that no member misses a post. Every post now goes to every other member; mentions only link a name to its bot. The follow-up cap still bounds bots answering each other between the user's posts; it is now decided once per post, since spending it per recipient shrank it with group size (six bot posts in a three-bot group). Posts past the cap are saved and reach members with the next delivered post.

Verification: the plain-tag regression fails under the old rule; bot tests pass 40 times plain and 10 under race; full Go tests pass.

## Only Mentioned Members Answer

- [x] Find why the Researcher also answered "give me TLDR [@CEO]" in "Business Research".
- [x] Give a turn only to the members a post mentions, and to every other member when it mentions nobody, from the user or a bot.
- [x] Keep every member's context: posts it was not given reach it with its next turn, uncapped.
- [x] Make a mention a handoff in the prompt: answer every post that mentions you, even mid-work, and mention everyone who must act.

Every post started a turn for every member, so both bots took the post at 10:33:38–40; CEO posted at 10:33:47 and the Researcher at 10:33:52, before it had seen CEO's answer. CEO, which did see the Researcher's post, stayed quiet, so "answer only when you add something new" works only when members are not answering at once. Grok Bot's rooms (0.66 bundle) work the same way: a member turn carries the messages since that member's last turn, and a turn ends SENT, PASS or SKIPPED.

A post now gives a turn to the members it mentions, or to every other member when it mentions none; a member mentioning only itself counts as mentioning none. The others' read position stays put, so their next turn opens with every post since. The 20-post cap on that catch-up is gone, since it dropped the posts a member had sat out. The 8 October plain `@CEO` post now counts as mentioning nobody and reaches everyone. The group prompt and every delivery say to answer each post that mentions you, and to mention everyone who must act, including the member you are answering, because a post with mentions gives nobody else a turn.

Verification: full Go tests pass; bot tests pass 40 times plain and 10 under race. Linked and named mentions, the post-to-all fallback, the self-mention rule and the uncapped catch-up each fail the new regression when removed, on the expected assertion.

### Requested Strict Review

- [x] Audit the change for structure, duplicated rules and reach into existing threads.
- [x] Fix confirmed findings, verify and commit.

Codex keeps the instructions a thread started with (`promptPersistsOnRestore`), so existing Codex group threads, including the Researcher's, never see a new group identity. The per-delivery line carried only "answer posts that mention you", not the rule that a post with mentions gives nobody else a turn, which is what keeps a handoff chain from skipping a member. The same rules were also written twice, in the identity and the delivery line. Now the identity states how the group works (who a post gives a turn to, and that skipped posts arrive with the next turn), and every delivery states what to do: answer each mention before the turn ends, post about the rest only to add something new, and mention everyone who must act. Routing code and tests were unchanged by the review; the routing in `post` is one condition in the layer that owns delivery, and no file approaches 1k lines.

## Teaching A Bot

- [x] When the user teaches a bot something or asks it to remember, save it in the bot's home so its group chats have it too.
- [x] Tell a group thread to reread AGENTS.md when the bot was taught after the group thread last saw the group.

A bot's own chat and its group threads are separate agent sessions in one home, and the agents load the home's AGENTS.md natively: Claude Code 2.1.289 as project instructions, but only when no CLAUDE.md sits above the home (none does under `~/.jaz/bots`); Codex as its AGENTS.md instructions. They load it when a session starts. Claude reloads it at its next session start; a Codex routine turn ran on the old copy 16 hours after an edit, and the new one arrived about 7 hours later. The home section of both identities now makes the home the bot's memory: taught rules and facts go in AGENTS.md, longer material in files AGENTS.md lists, saved before the bot says it is saved. A group delivery opens with a reread note when AGENTS.md changed after the last group message the member was shown; a new group thread loads the current file itself. Taught in a group, the bot's own chat sees it from its next session start. Codex threads keep the identity they started with, so existing Codex bots keep the old home section; the reread note reaches them.

Verification: full Go tests and vet pass; bot tests pass 30 times plain and 10 under race. The regression fails when the change check always reports a change (unchanged file flagged) and when it never does (teaching missed). Live Claude Code probe in a home with the same parent chain as `~/.jaz/bots`: told to remember three facts, the 1:1 session wrote them to AGENTS.md before confirming; a fresh session with the group identity and every file tool blocked answered all three.

### Requested Strict Review

- [x] Audit the change for structure, duplicated rules and naming.
- [x] Fix confirmed findings, verify and commit.

The group identity restated, in one clause, what the shared home section says in full; the clause is gone. The note constant is named for what it asks (reread). Kept: the time of the last shown message returned by `unseen`, its only caller, since a stored column or a second event scan adds more; the cwd guard, because delivery runs in a goroutine and an empty cwd would stat the server's own directory; and the note composed once in `deliverLocked` rather than a flag on both prompt builders.

Verification: go vet and bot tests pass 20 times plain and 5 under race.
