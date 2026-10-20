# iOS Push Delivery

The paired Jaz server sends bot messages, chat completion alerts, questions and Live Activity updates directly to Apple. No relay service or new application dependency is required.

## Configuration

Set these server configuration values, then restart the server when its active turns have finished:

- JAZ_APNS_KEY_FILE: path to the Apple APNs signing key (.p8), kept outside the repository.
- JAZ_APNS_KEY_ID: the key's Apple identifier.
- JAZ_APNS_TEAM_ID: the Apple Developer team identifier.
- JAZ_APNS_BUNDLE_ID: defaults to jazresearch.jaz.

Equivalent configuration fields are jaz.push.keyfile, keyid, teamid and bundleid. Empty signing credentials leave push disabled; the iOS settings screen reports this.

## Contract

- Device authentication binds registrations to the requesting paired device. Root credentials cannot register a phone.
- GET/PUT/DELETE /v1/mobile/push reports configuration, registers notifications or opts out. Opting out preserves Live Activities.
- PUT/DELETE /v1/mobile/activities/{activity} registers or removes a conversation's ActivityKit token.
- Notification payloads carry the conversation kind, session ID and per-server installation ID. Questions additionally carry their request ID. Secret questions open the app.
- Bot notifications use explicit room messages; private narration is excluded. Chat completion follows saved turn bookkeeping. Group progress uses the same member-turn calculation as the bot API.
- Progress updates have increasing Apple timestamps, coalesce unchanged state and refresh once per minute. Expired tokens are removed; revoked devices receive no pushes. Apple transport failures retry with bounded timeouts.

## Verification and Review

Full backend suite and vet pass. The push packages pass the race detector; the registration/completion worker passed 50 ordinary repetitions. Frontend: 339 tests pass. A real HTTP router starts and serves the new configuration endpoint.

HTTP/2 tests verify ES256 signing, APNs headers, empty success responses, token invalidation and transient-failure retry. Real SQLite tests cover message eligibility, saved completion, questions, opt-out, revocation, deduplication, monotonic progress and late activity registration. Overlay controls fail for the expected revoked-device and non-bot-message violations.

Thermo-nuclear self-review consolidated message eligibility at one boundary, narrowed activity request DTOs, kept pending-question state with the agent manager and kept signing, registration, delivery, worker lifecycle and HTTP handling separate. No provider prompt, tool, auth or execution policy changes.

Actual APNs delivery requires the operator's Apple signing key and a provisioned iOS build. It has not been exercised against Apple's live service.
