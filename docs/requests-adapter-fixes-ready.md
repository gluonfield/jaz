# Unattended Claude Prompt Suggestions

Claude sessions used for bot workers, group seats, routine runs and memory jobs
start and resume with the native SDK option `promptSuggestions: false`. Ordinary
user threads and a bot's main chat retain the provider default. This works with
the existing Claude adapter and does not change prompts, tools or authentication.

The session's persisted source type determines the option on both initial launch
and restart. A regression test covers eight source classes across a Manager/store
restart. Four compiled negative controls catch incorrect source classification
and omitted source information on launch or resume.

## Measured Behavior

On 19 October 2026, four ordinary coding prompts using Opus 5.5 subscription OAuth
and Claude Code 2.1.300 produced the following native prediction API requests:

| Session | Before | After |
| --- | ---: | ---: |
| Unattended worker | 3 | 0 |
| Visible user thread | 2 | 3 |

Counts use native API-request records with `source=prompt_suggestion`.
Native conversation and cache gates affect eligibility; the visible-session
comparison confirms predictions remain available.

Hidden turns inside a main bot chat remain unresolved. The native pause control
was acknowledged but rollout-gated; changing the session flag settings did not
update the running prediction state. Both experimental controls were removed
after live checks. No new Claude adapter version is required by this change.

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
