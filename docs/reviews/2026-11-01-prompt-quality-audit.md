# Jaz agent quality: prompt audit, 25 October–1 November 2026

The screenshot shows unnecessary native-agent coordination in a regular Jaz thread. The actual bot-chat coordinator prompt was not active. I incorrectly applied a bot-only rule from shared personal instructions. This thread stayed on Sol/xhigh; the evidence does not establish a general decline in the underlying model. [Prompt selection](../../backend/internal/app/app.go), [Codex launch configuration](../../backend/internal/acp/codex_config.go)

## Why the screenshot shows agent activity

1. The launch prompt included the personal `AGENTS.md` paragraph headed `Jaz Bots`: every Jaz bot coordinates and delegates engineering, however small. Its scope was bots, not ordinary work threads. The platform injects the whole personal file into both contexts. [Shared-file assembly](../../backend/internal/coordinator/prompt.go)
2. Codex's native agent tooling permitted delegation when applicable instructions called for it; it did not require this sound change to be delegated. The screenshot's `/root/contents_diagnosis` was an existing native child, not a new bot chat or a separate saved Jaz thread. [Thread/native-agent distinction](../../backend/internal/templates/jazplatform/jazplatform.tmpl)
3. For Blow, the visible native trace records a `followup_task` at 09:20:57, an administrative `send_message` at 09:21:09, and two `wait_agent` calls at 09:21:11 and 09:21:28. The worker added the two notification sound options. Those follow-ups and waits explain the activity rows; none was necessary for the edit. [Notification implementation](../../frontend/src/main/notifications.ts)
4. The failure was treating the bot-only paragraph as applicable, then continuing an unnecessary coordinator/worker workflow. Existing bot-module routing was correct. The missing explicit session-role label was a prompt weakness; it does not prove that every model would make the same mistake. [Existing routing coverage](../../backend/internal/app/bot_prompt_test.go), [Role-label fix](../../backend/internal/coordinator/prompt.go)

## Findings and changes

**1. Legacy skill copies are accepted and left unchanged.** Commit `5d8cfce1` stopped copying Jaz skills into native CLI profiles on 24 October and left installed copies behind. This thread's native prompt repeated 34 skill names, but no causal link to the delegation was established. The user confirmed that leaving existing copies is acceptable. Automatic retirement has been removed from the proposed fix. [Catalogue contract](../../backend/README.md), [Skill loading](../../backend/internal/skills/skills.go)

**2. The shared prompt did not identify the active session role.** The existing router correctly limited bot coordinator instructions to bot chats. However, ordinary threads also saw personal instructions containing bot-only rules without an explicit role label. I misapplied such a rule in this conversation. The fix carries the stored source type into prompt assembly on launch, resume and native requests. Regular threads and bot-spawned threads receive the same work-thread role; bot chats receive the bot-chat role. [Existing separation](../../backend/internal/app/bot_prompt_test.go), [Prompt assembly](../../backend/internal/coordinator/prompt.go), [Shared prompt](../../backend/internal/templates/jazplatform/jazplatform.tmpl)

**3. Personal-file instructions encouraged over-eager edits.** The older shared prompt said to write immediately and that the files did not wait for the user to ask. That wording existed before this week, but was directly relevant to my mistaken personal `AGENTS.md` edit. The replacement distinguishes explicit lasting personal instructions from requests to change Jaz, its prompts, a project or a generated document. The latter belong in project files. Personal instructions themselves were not changed by this fix. [Updated boundary](../../backend/internal/templates/jazplatform/jazplatform.tmpl)

**4. Memory lookup quality can change independently of the main model.** Saved memory-search records show Luna/low on 25 and 27 October, Astra/xhigh on 29 October, and Luna/medium on 31 October and 1 November. Recent lookups in this conversation used Luna/medium. The 31 October commit `5b53a6c1` changed unspecified memory-worker defaults to Luna/low and tightened selection of the requested model; explicit settings still take precedence. These receipts do not establish whether the observed switch came solely from the default change or from saved settings. Weaker lookup answers could affect any primary agent using the memory tool. No memory-model setting was changed in this fix. [Worker defaults](../../backend/internal/settings/agents.go), [Explicit settings](../../backend/internal/settings/memory.go), [Memory-search runner](../../backend/internal/memorysearch/runner.go)

**5. Broad context is an observation, not an established cause.** This thread's first native request reported 39,877 input tokens overall, including native instructions, tools, user input and Jaz context. Its launch extension contained approximately 66,600 characters of Jaz context plus a 29,200-character native skill catalogue. Daily memory is also injected whole: the 25 October file currently measures 127,631 characters. This fix does not change skill copies, memory injection or provider-owned context/compaction behavior. [Memory injection](../../backend/internal/coordinator/memory.go), [Platform assembly](../../backend/internal/coordinator/prompt.go)

## What changed this week

| Date | Change | Relevance |
|---|---|---|
| 25 October | Shorter bot handoffs (`66931f50`, `f0a5a98b`) | [Bot coordination](../../backend/internal/bots/prompt.go); no ordinary-thread model downgrade. |
| 28 October | Stricter, shared bot delegation instructions (`d094131f`) | Bot chats only; [integration coverage](../../backend/internal/app/bot_prompt_test.go) checks ordinary and bot-spawned threads receive no bot modules. |
| 29–31 October | Markdown delivery, artifact and brevity guidance (`d07fdeda`, `98d41164`, `9d874dc7`, `41327f0b`, `517bc793`) | [Shared authoring behavior](../../backend/internal/templates/jazplatform/jazplatform.tmpl) changed. No demonstrated link to false architectural claims. |
| 31 October | Memory-worker defaults/selection (`5b53a6c1`) | [Worker settings](../../backend/internal/settings/agents.go) can change supporting lookup and memory-maintenance behavior independently of the main model. |

The current thread's native records report `gpt-6.1-sol`, `xhigh`, and Codex CLI `0.164.0`. The inspected parent-session samples from 25 October–1 November also report CLI `0.164.0`. The inspected launch/model configuration has no silent Sol reasoning downgrade to explain these mistakes. [Launch model and effort](../../backend/internal/acp/codex_config.go), [Prompt delivery](../../backend/internal/acp/prompt_policy.go)

## Verification and remaining decisions

The new role checks failed against the old prompt, then passed after the fix. They exercise native request construction and actual Claude ACP launch/resume requests. Existing Codex launch/auth/prompt-restoration and native-profile preservation checks protect provider behavior. These checks validate composition; they do not measure improved model answers. [Role regressions](../../backend/internal/acp/bot_prompt_refresh_test.go), [Native requests](../../backend/internal/jazagent/prompt_role_test.go), [Profile preservation](../../backend/internal/acp/process_skills_test.go)

Go 1.26 verification covers the prompt paths and unchanged native-profile behavior. Thermo-nuclear self-review keeps the fix at the stored-session/prompt boundary, without automatic profile changes. [Prompt checks](../../backend/internal/acp/bot_prompt_refresh_test.go), [Profile checks](../../backend/internal/acp/process_skills_test.go)

- [ ] Merge/release the verified prompt fixes; fresh sessions receive explicit role and personal-file boundaries. Existing provider histories and native profiles are not rewritten.
- [ ] Evaluate memory-model quality independently if lookup accuracy remains a problem; no setting change or user decision is needed for the delegation diagnosis.
- [ ] Compare matched native/Jaz tasks on the same model and reasoning effort, including the role-confusion and personal-file cases, before claiming that the broader quality problem is resolved.
