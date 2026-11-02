# iOS Push Delivery

The paired Jaz server sends conversation alerts and Live Activity updates directly to Apple. No relay service or new application dependency is required. Desktops receive the same alerts over a live stream.

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
- One classifier in `internal/mobilepush/alert.go` decides alerts for iPhones and desktops from typed state, never message wording: done (a bot message that delivered a task), reply (a plain thread's final reply, or a bot's final message in a person-started turn that opened no task), needs_you (a task question or an agent question) and stopped (a stopped task or a failed turn). Acknowledgements, progress, cancels, restarts, compactions and child turns are silent. A group member's reply alerts as its group, judged by that member's own newest post. A turn the person writes into while it runs, directly or through their group post, answers them as one they started does.
- A question opens the thread holding it, where it is answered. One from a thread that does not alert on its own (a child, a bot's worker, a group member's thread or a routine run) asks as the plain thread or bot it was started for, with that bot's name and face.
- Bots tag the messages that change tasks (`RoomMessageEvent.Task`) and the person-started turn they answer (`Answers`); a commitment records the turn that opened it. Private narration is excluded. Group progress uses the same member-turn calculation as the bot API.
- Alerts show the thread or task title, with the bot or group name as subtitle, and the reply's first sentence as plain text.
- GET /v1/notifications/stream sends alerts decided while a desktop listens as server-sent events, with a 25-second heartbeat. Nothing is replayed after a reconnect. The desktop shows each alert once, beside the notch or in Notification Center, and nothing while a Jaz window is focused.
- The notch island needs notifications allowed and Focus known off. The `jaz-notch` helper reads both without prompting; it is signed under Jaz's bundle identifier, because macOS refuses settings reads from code signed as another identity and answers with defaults. macOS is asked to share Focus only when the person chooses a notch mode or presses Share Focus, and only while undecided.
- Progress updates have increasing Apple timestamps, coalesce unchanged state and refresh once per minute. Expired tokens are removed; revoked devices receive no pushes. Apple transport failures retry with bounded timeouts.

## Verification and Review

Full backend suite and vet pass. The push packages pass the race detector; the registration/completion worker passed 50 ordinary repetitions. Frontend: 339 tests pass. A real HTTP router starts and serves the new configuration endpoint.

HTTP/2 tests verify ES256 signing, APNs headers, empty success responses, token invalidation and transient-failure retry. Real SQLite tests cover message eligibility, saved completion, questions, opt-out, revocation, deduplication, monotonic progress and late activity registration. Overlay controls fail for the expected revoked-device and non-bot-message violations.

Thermo-nuclear self-review consolidated message eligibility at one boundary, narrowed activity request DTOs, kept pending-question state with the agent manager and kept signing, registration, delivery, worker lifecycle and HTTP handling separate. No provider prompt, tool, auth or execution policy changes.

Actual APNs delivery requires the operator's Apple signing key and a provisioned iOS build. It has not been exercised against Apple's live service.

## Follow-Up Review

The explicitly requested 2026-10-20 thermo-nuclear review fixed stale delivery results deleting a newly rotated Live Activity token. Delivery completion now updates or removes only the registration snapshot actually sent. A SQLite regression rotates the token while delivery is in flight and exercises both successful completion and invalidation; removing the token comparison makes it fail for the expected reason.

The push service is a required route dependency even when Apple credentials are absent; configuration status remains available and missing application wiring fails at construction. Full backend tests, vet, transport race checks and 50 repetitions of rotation and late-registration checks pass. Previous revocation and explicit-message controls also pass. The iOS review record is docs/ios-system-integrations.md in the iOS repository.
