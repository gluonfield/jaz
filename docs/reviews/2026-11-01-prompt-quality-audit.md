# Jaz agent quality: prompt audit, 25 October–1 November 2026

There are concrete prompt/context defects to fix, and a recent change in memory-worker models worth investigating. This thread stayed on Sol/xhigh; the evidence does not establish a general decline in the underlying model. Bot chats already had a separate coordinator prompt. [Prompt selection](../../backend/internal/app/app.go), [Codex launch configuration](../../backend/internal/acp/codex_config.go)

## Findings and changes

**1. Old skill copies survived the 24 October fix.** Commit `5d8cfce1` stopped copying Jaz skills into native CLI profiles, but left installed copies behind. This thread's native prompt repeated 34 skill names between Jaz's catalogue and Codex's own catalogue. Both managed Codex and Claude profiles contain 63 overlapping `SKILL.md` files: 62 match current originals and one differs. A read-only directory comparison identified 33 matching directory candidates. This is duplicated context across both profiles; its effect on answer quality has not been measured. [Original catalogue contract](../../backend/README.md), [Skill loading](../../backend/internal/skills/skills.go)

The fix compares entire directory trees, including permissions, before moving exact copies into `retired-jaz-skills` outside the native discovery directory. It runs only for Jaz-managed Codex/Claude profiles. It preserves edited directories, existing backups, symlinks and external CLI profiles. No live profile was changed during this audit. [Retirement implementation](../../backend/internal/skills/profile.go), [Profile integration](../../backend/internal/acp/process.go), [Filesystem regressions](../../backend/internal/skills/profile_test.go)

**2. The shared prompt did not identify the active session role.** The existing router correctly limited bot coordinator instructions to bot chats. However, ordinary threads also saw personal instructions containing bot-only rules without an explicit role label. I misapplied such a rule in this conversation. The fix carries the stored source type into prompt assembly on launch, resume and native requests. Regular threads and bot-spawned threads receive the same work-thread role; bot chats receive the bot-chat role. [Existing separation](../../backend/internal/app/bot_prompt_test.go), [Prompt assembly](../../backend/internal/coordinator/prompt.go), [Shared prompt](../../backend/internal/templates/jazplatform/jazplatform.tmpl)

**3. Personal-file instructions encouraged over-eager edits.** The older shared prompt said to write immediately and that the files did not wait for the user to ask. That wording existed before this week, but was directly relevant to my mistaken personal `AGENTS.md` edit. The replacement distinguishes explicit lasting personal instructions from requests to change Jaz, its prompts, a project or a generated document. The latter belong in project files. Personal instructions themselves were not changed by this fix. [Updated boundary](../../backend/internal/templates/jazplatform/jazplatform.tmpl)

**4. Memory lookup quality can change independently of the main model.** Saved memory-search records show Luna/low on 25 and 27 October, Astra/xhigh on 29 October, and Luna/medium on 31 October and 1 November. Recent lookups in this conversation used Luna/medium. The 31 October commit `5b53a6c1` changed unspecified memory-worker defaults to Luna/low and tightened selection of the requested model; explicit settings still take precedence. These receipts do not establish whether the observed switch came solely from the default change or from saved settings. Weaker lookup answers could affect any primary agent using the memory tool. No memory-model setting was changed in this fix. [Worker defaults](../../backend/internal/settings/agents.go), [Explicit settings](../../backend/internal/settings/memory.go), [Memory-search runner](../../backend/internal/memorysearch/runner.go)

**5. Broad context remains a plausible contributor.** This thread's first native request reported 39,877 input tokens overall, including native instructions, tools, user input and Jaz context. Its launch extension contained approximately 66,600 characters of Jaz context plus a 29,200-character native skill catalogue. Daily memory is also injected whole: the 25 October file currently measures 127,631 characters. These are context costs, not proof of causation. This fix removes redundant skill copies while preserving memory and provider-owned context/compaction behavior. [Memory injection](../../backend/internal/coordinator/memory.go), [Platform assembly](../../backend/internal/coordinator/prompt.go)

## What changed this week

| Date | Change | Relevance |
|---|---|---|
| 25 October | Shorter bot handoffs (`66931f50`, `f0a5a98b`) | [Bot coordination](../../backend/internal/bots/prompt.go); no ordinary-thread model downgrade. |
| 28 October | Stricter, shared bot delegation instructions (`d094131f`) | Bot chats only; [integration coverage](../../backend/internal/app/bot_prompt_test.go) checks ordinary and bot-spawned threads receive no bot modules. |
| 29–31 October | Markdown delivery, artifact and brevity guidance (`d07fdeda`, `98d41164`, `9d874dc7`, `41327f0b`, `517bc793`) | [Shared authoring behavior](../../backend/internal/templates/jazplatform/jazplatform.tmpl) changed. No demonstrated link to false architectural claims. |
| 31 October | Memory-worker defaults/selection (`5b53a6c1`) | [Worker settings](../../backend/internal/settings/agents.go) can change supporting lookup and memory-maintenance behavior independently of the main model. |

The current thread's native records report `gpt-6.1-sol`, `xhigh`, and Codex CLI `0.164.0`. The inspected parent-session samples from 25 October–1 November also report CLI `0.164.0`. The inspected launch/model configuration has no silent Sol reasoning downgrade to explain these mistakes. [Launch model and effort](../../backend/internal/acp/codex_config.go), [Prompt delivery](../../backend/internal/acp/prompt_policy.go)

## Verification and remaining decisions

The new role checks failed against the old prompt, then passed after the fix. They exercise native request construction and actual Claude ACP launch/resume requests. Filesystem checks exercise retirement, idempotence, preserved custom files/backups, symlink boundaries and native-profile isolation. Existing Codex launch/auth/prompt-restoration checks protect provider behavior. These checks validate composition and filesystem behavior; they do not measure improved model answers. [Role regressions](../../backend/internal/acp/bot_prompt_refresh_test.go), [Native requests](../../backend/internal/jazagent/prompt_role_test.go), [Profile isolation](../../backend/internal/acp/process_skills_test.go)

Go 1.26 verification passed: 52 focused tests with the race detector, plus `go vet` and `git diff --check`. Thermo-nuclear self-review identified the symlink boundaries; those are guarded and covered by filesystem regressions. [Retirement implementation and checks](../../backend/internal/skills/profile_test.go)

- [ ] Merge/release the verified fixes; fresh sessions then receive the new prompt and profile cleanup. Existing provider histories are not rewritten.
- [ ] Decide whether memory search/dream should retain Luna/medium or use a stronger model; this is a separate quality/cost choice.
- [ ] Compare matched native/Jaz tasks on the same model and reasoning effort, including the role-confusion and personal-file cases, before claiming that the broader quality problem is resolved.
