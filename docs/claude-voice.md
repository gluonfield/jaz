# Claude Voice Integration

Request, 25 October 2026: inspect Claude Desktop's voice implementation and add Claude voice to Jaz.

- [x] Inspect installed Claude Desktop and its cached first-party web client.
- [x] Trace Jaz's current provider, connection and transcript boundaries.
- [x] Check the existing Claude Code sign-in without generating a completion.
- [ ] Verify the signed-in Claude voice handshake and actual model identity.
- [ ] Implement and verify the Claude provider, including delegation and interruption.
- [ ] Review and commit the verified implementation.

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

Both Jaz `browser_status` and `computer_status` report disconnected for this conversation. Augustinas has been asked to open it in Jaz Desktop. Resume by observing Claude's signed-in voice session, verifying the handshake, authentication and model identity, then implementing the smallest measured adapter. Production code is unchanged; there is no Claude voice support yet.

Official feature documentation: [Use voice mode](https://support.claude.com/en/articles/11101966-use-voice-mode).
