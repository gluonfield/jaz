# Claude Prompt Suggestions in Bot and Background Sessions

Claude sessions used for bot chats, bot workers, group seats, routine runs and
memory jobs start and resume with the native SDK option `promptSuggestions: false`.
Ordinary user threads retain the provider default. Human-started and hidden turns
in a bot's chat share the same session policy. This works with the existing Claude
adapter and does not change prompts, tools or authentication.

The session's persisted source type determines the option on both initial launch
and restart. Regression tests cover eight source classes and six other agents
across a Manager restart, sending both human and hidden turns. The bot-chat case
fails against the old policy on both initial launch and resume.

## Historical Worker Measurement

On 19 October 2026, four ordinary coding prompts using Opus 5.5 subscription OAuth
and Claude Code 2.1.300 produced the following native prediction API requests:

| Session | Before | After |
| --- | ---: | ---: |
| Unattended worker | 3 | 0 |
| Visible user thread | 2 | 3 |

Counts use native API-request records with `source=prompt_suggestion`.
Native conversation and cache gates affect eligibility; the visible-session
comparison confirms predictions remain available.

The earlier attempt to pause individual hidden bot turns was rollout-gated.
The current policy disables suggestions when opening the bot session instead;
it does not use the experimental pause control. No new live Claude probe or
adapter version is required by this change.

## Separate Codex Release Pin

The Codex 2.2.2-jaz.3 manifest update lives only on
`jaz/codex-acp-2.2.2-jaz.3-pin` until that release is published. The quiet-suggestions
branch does not change adapter pins. Six prepared archives match the proposed
checksums; macOS arm64 was executed and the other platform binaries were inspected.
The earlier native/ACP skill-instruction size discrepancy remains a release gate.

The Codex candidate passed 1,474 tests, typechecking and three compiled regression
controls. Scratch Jaz checks with the actual runtime and a local provider
stand-in covered startup, same-session restart, steered failure and expired-login
preservation. These checks do not constitute a new full native-parity comparison.
