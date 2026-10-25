# Claude Voice Integration

Request, 25 October 2026: inspect Claude Desktop's voice implementation and add Claude voice to Jaz.

- [x] Inspect installed Claude Desktop and its cached first-party web client.
- [x] Trace Jaz's current provider, connection and transcript boundaries.
- [x] Check the existing Claude Code sign-in without generating a completion.
- [x] Reproduce, repair and review authenticated desktop WebSocket negotiation.
- [x] Activate the reviewed connection correction with a coordinated backend restart.
- [x] Verify signed-in WebSocket/WebRTC signaling and detect the model-ID mismatch.
- [x] Verify the current native Opus voice mapping and inspect the reported live model.
- [ ] Resolve the live model identity mismatch and verify real audio/delegation.
- [x] Implement the Claude provider and verify signaling, delegation and cancellation against real protocol fixtures.
- [x] Review and commit the locally verified implementation.
- [x] Resolve the requested merge from current main and verify the combined branch.
- [ ] Activate the provider and complete the live Opus/audio/handoff check.

## Evidence

Claude Desktop 3.6556.2 loads its chat implementation from the web. Its local `app.asar` contains the Electron shell and bundled Claude Code, including dictation settings. The voice transport is in first-party JavaScript cached under `~/Library/Application Support/Claude/Cache/Cache_Data`. Only public JavaScript assets were extracted; browser cookies and account data were not copied.

The cached client builds these conversation-scoped WebSocket paths:

```
/api/ws/voice/organizations/{organization}/chat_conversations/{conversation}
/api/ws/voice/webrtc/organizations/{organization}/chat_conversations/{conversation}
```

The first carries socket audio. The second carries signaling, with WebRTC carrying audio. It advertises `webrtc_media`; `session_server_initialized` must announce protocol `2.1` and media `webrtc`. The client sends `media_offer` with `data.sdp` and receives `media_answer`. Its peer uses an audio transceiver and no OpenAI data channel.

The client sends voice, language, model and audio configuration in the connection query. The captured default is `buttery`, with socket input Opus at 16 kHz, mono, and PCM output. These are observations of cached assets, not a verified current server catalog.

Control and transcript events include `close_stream`, `transcription_start`, `user_input_end`, `message_sse`, `message_complete`, `playback_start`, `playback_stop`, `server_interrupt` and `text_reply_end`. WebRTC and socket audio have different playout handling.

The `client_tools` capability permits `tools_register`. The host posts tool results to `/api/organizations/{organization}/chat_conversations/{conversation}/tool_result`. The client can send `user_message` with `data.text`, optional `kind` and `respond`. Their use for Jaz context/delegation still needs a live contract check.

Separate dictation and read-aloud paths also exist: `/api/ws/speech_to_text/voice_stream` and `/api/ws/text_to_speech/text_stream`. Dictation has a ticket-authenticated transport option; this does not establish a ticket route for conversational voice.

Source files and SHA-256 hashes are recorded in ignored `runs/claude-voice/source-manifest.json`. Relevant assets are `c3e5e55a7-Cpqjg5cU.js` (connection), `ccc68ce7f-OeSGrCUJ.js` (session and tools), `cd27cb2c4-BKjGanYM.js` (dictation) and `cb17312fe-ndIJ7qYH.js` (read aloud). Fetching their public URLs returned HTTP errors, so the evidence remains the installed app's cache rather than a freshly fetched release.

## Authentication Check

The existing Jaz Claude Code OAuth token was unexpired and returned HTTP 200 from `https://api.anthropic.com/api/oauth/profile`. An upgrade request to the observed voice path returned HTTP 403 from the API host; the Claude web host returned HTTP 403 with `cf-mitigated: challenge`. A deliberately nonexistent conversation was used; no audio or prompt was sent, and no conversation was created.

These results establish that a working voice connection has not been demonstrated with the native sign-in. They do not distinguish every upstream authorization or routing policy. The captured browser client uses `new WebSocket(url)` without a bearer token, consistent with browser-session authentication.

Desktop bootstrap metadata maps Opus 5.5's voice model to `claude-opus-5`. That mapping is not proof of the served model identity. Verify it before selecting a model, preserving Augustinas's Opus 5.5 rule.

## Jaz Implementation Boundary

`backend/internal/voice/live/settings.go` currently accepts only OpenAI OAuth/API providers. `service.go` creates an HTTP SDP session, and `frontend/src/renderer/src/lib/voice/connection.ts` creates the OpenAI event data channel. Claude needs its own signaling adapter feeding the existing transcript, admission and task-stream contracts. Keep provider wire events at that adapter boundary and preserve native agent authentication and prompting.

A Claude connection must work with a remote Jaz server and local clients. Reading a client-local Claude cookie database from the server is not a deployment design. Establish the actual authentication contract before adding credential fields or a new sign-in flow.

## Pending Live Check

Augustinas confirmed that this conversation is already open in Jaz Desktop and Claude Desktop is also open. Both Jaz tools still report disconnected. Read-only checks confirm Browser is enabled in desktop mode and Computer use is enabled. An authenticated MCP check with this thread's explicit session ID returns the same disconnection, so this is not evidence of a missing session header. The HTTP handlers and frontend hooks use the same canonical session identity; no concrete attachment defect has been established.

Asked Augustinas to reload the Jaz window once with Cmd-R to remount the desktop connections. That did not restore attachment. The signed-in Claude handshake, authentication and model check remain pending; there is no Claude voice support yet.

## Desktop Connection Fix

Reloading did not restore the connection. Investigation found a concrete transport defect: authenticated browser clients offer `jaz-auth.{credential}` as their only WebSocket subprotocol, while the browser, computer and terminal HTTP handlers acknowledge no subprotocol. The WebSockets browser standard requires a selected protocol when the client offers protocols, so Chromium closes the connection immediately. The previous backend security test explicitly expected an empty response header and used a Go client that accepts it.

The correction offers a public `jaz` protocol alongside the request-only credential and uses one shared HTTP upgrader to select `jaz` for all three endpoints. Authentication, origin policy, credential storage and native agent behavior remain unchanged. Existing terminal revocation and browser endpoint tests now require the public protocol and reject credential echoing.

A normal-path hidden Electron security smoke uses the production browser transport and HTTP adapter. It failed on the original code with `Authenticated browser socket handshake failed`; it passes with the correction and verifies registration with a real status request. The test creates no visible windows or focus changes. Full Go 1.26.3 backend tests/vet, the frontend's 376 tests, type checks, lint and desktop bundle pass. The thermo-nuclear self-review found no remaining blockers: one HTTP owner replaces three upgrader configurations, credentials remain request-only, and provider/native-agent behavior is unchanged. Logs: `runs/claude-voice/{handshake-before,handshake-after,backend-tests,backend-vet,frontend-tests,typecheck,lint,build}.log`.

Augustinas authorized the shared restart. The correction was merged into local main as `dd1e8b1b`, built with Go 1.26.3 and activated by a graceful backend replacement. The receipt reports healthy, exact prior environment reused and unchanged auth file. The side browser now connects to this conversation, confirming the production handshake correction. Computer use still reports disconnected. Claude voice itself remains unimplemented.

Claude in the Jaz side browser shows its sign-in page; the installed Desktop session is separate. Requested sign-in there to verify the real voice authorization and model contract. No Claude credentials were extracted, copied or stored. Restart receipt: main checkout `runs/claude-voice/restart-receipt.json`. The replacement backend was started detached using the prior environment; its PID is registered in the existing Electron orphan-cleanup PID file.

Official feature documentation: [Use voice mode](https://support.claude.com/en/articles/11101966-use-voice-mode).

## Signed-in Probe and Implementation

After Augustinas signed in, the browser returned organizations HTTP 200 and created a temporary test conversation (201). A WebRTC voice socket initialized protocol 2.1, accepted an SDP offer and connected ICE/media. A synthetic silent track avoided capturing Augustinas's microphone. The text-only request "Reply with the single word Ready." returned "Ready.".

The request used the text ID `claude-opus-5-5`, but `message_start` reported `claude-sonnet-5`. The live native voice catalog instead lists Opus as `claude-opus-5`, matching the installed client's explicit text-to-voice mapping. Requested clarification before generating with that voice-specific ID, because Augustinas's standing Anthropic model rule names Opus 5.5. No further completion was sent. A valid-conversation native Code OAuth upgrade still returned 403 on both hosts. Sanitized evidence: `runs/claude-voice/live-probe.json`.

The provider implementation keeps browser authentication on the local desktop in the existing browser partition. A sandboxed hidden broker performs same-origin HTTP/WebSocket requests and exposes no privileged API to the remote page. Jaz's server stores the provider/voice/model choice and supplies existing companion context; it receives no Claude browser credential. The client uses Claude's live model catalog and requires an explicit selection. Separate OpenAI and Claude signaling adapters share the existing media, transcript, admission and agent-stream flow. A returned model mismatch stops the call.

The code passes the checks below and is ready for integration. No Claude provider changes are active in main yet. Native Opus, real microphone/playback and live Jaz handoff remain unverified. The previously activated WebSocket fix remains separate. The side browser disconnected again after another runtime replacement; Augustinas need not repeat the already-completed sign-in.

## Verification and Review

- Go 1.26.3 voice/live and HTTP voice tests; repository-wide Go vet.
- 378 frontend tests, both type checks, lint and the desktop bundle.
- Normal hidden Chromium suite (`test-browser --claude-voice`): same-origin cookie HTTP/WebSocket auth, live catalog filtering, client-tool result shape, observation isolation, startup cancellation, owner reload cleanup, cross-window denial, and desktop/phone settings selection. The broker stays hidden/unfocused and exposes neither Jaz IPC nor Node APIs to the remote page. Final evidence: `runs/claude-voice/broker-smoke.log`, captures in `runs/browser-smoke-VLU3jh/`.
- Thermo-nuclear self-review: fixed concurrent status-request disposal, owner-reload cleanup, unknown-call delivery failure and invalid-input allocation. Kept microphone activation at the user action and shared media/admission rather than duplicating it. No remaining structural findings. Native agent metadata, prompts, tools, compaction and authentication are unchanged.
- A broadened full-backend test run was interrupted when its native turn was cancelled; it supplies no full-suite pass claim. The focused affected packages and complete vet finished successfully.

## Use

On Jaz Desktop, choose Settings → Voice → Provider → Claude, sign in to Claude in the side browser if needed, then explicitly choose a model from Claude's live voice catalog. The selected coding agent continues to handle work through Jaz's existing admission stream. Browser-only clients show that this provider requires Desktop. The current OpenAI selection is preserved until changed explicitly.

## Main Merge Repair

Augustinas requested repair of the failed main merge. Merged main `91f9f05b`, preserving both Claude voice and the new connection-session preload APIs, plus both browser smoke selectors. Two existing browser checks used the removed `apiAuthToken` export; they now use `cachedApiAuthToken`, and the Claude check restores the actual in-memory API URL.

The combined branch passes all 404 frontend tests, both type checks, lint, the desktop bundle and four affected Go 1.26.3 packages. Hidden Electron checks pass for Claude signaling/settings, authenticated connection security and browser visibility. An initial parallel run collided on the system clipboard; the sequential browser-visibility run passes. Logs are in `runs/claude-voice/merge-main-20261025/`. Thermo-nuclear self-review found no remaining merge issues. This merges main into the feature branch only; provider activation and the native-model live check remain pending.

## Live Identity Check

At 22:03 UTC, the signed-in native `/model_selector/chat` metadata explicitly mapped text model `claude-opus-5-5` to voice ID `claude-opus-5`. This establishes the provider-owned request mapping; it does not establish the generation's identity. A temporary conversation and synthetic silent WebRTC track connected on protocol 2.1. The first `message_start` after a minimal text request reported `claude-opus-4-8`. The check immediately closed the socket and peer, stopped the synthetic source, and deleted its own temporary conversation (204). No user microphone was captured or output played. Evidence: `runs/claude-voice/live-acceptance/identity-check.json`.

The advertised native voice ID and the returned model ID differ, so the current model guard rejects this live response. Native Opus 5.5 identity, real audio and live Jaz delegation remain unverified. Asked Augustinas whether native Opus voice may handle speech despite its reported model, while the selected coding agent handles work. This is an explicit exception to his Opus 5.5-only Anthropic instruction; no answer is assumed. Main and the running app remain unchanged.
