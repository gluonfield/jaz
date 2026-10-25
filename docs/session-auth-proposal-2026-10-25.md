# Session Authentication Proposal

**For independent review; no policy implementation authorized yet.** `7f4ec279` remains diagnostic evidence, held from integration as the final fix. Success means usable authenticated Jaz with Keychain access denied, including local reconnect after an app restart.

## Proposed Default

Fresh backend credentials establish a session in the existing origin-keyed `authTokens` memory cache. Registration/enrollment/pairing and `/v1/auth/check` must still succeed before the connection is published. Install the verified device token and its resource ticket together, after the existing cancellation/generation checks; an unsuccessful or cancelled candidate never becomes an active credential.

Ordinary API, WebSocket, polling and notification access reads memory only. Reuse `cc44de8a`'s `cachedApiAuthToken`; isolate the existing disk/decrypt path behind explicit saved-connection restoration. Split cache installation from encrypted remembering instead of catching encryption errors and treating them as authentication success. No automatic encryption, decryption, legacy-token migration, ciphertext replacement or plaintext write occurs during a fresh session.

## Lifecycle And UI

| Event | Proposed behavior |
| --- | --- |
| **Run on this Mac** | Start/adopt the backend through the existing trusted local-start IPC. Once healthy, verify an already cached device token first. With an empty session cache, use the returned root grant only at its local registration endpoint, reuse the persistent device profile, verify the issued device token, then cache it. Never attempt an old encrypted local token before this path. A transient backend outage alone does not justify token reissuance. |
| Local network/backend interruption | Recheck the cached token after health recovers. A live 401/403 stops access; it does not trigger an automatic re-enrollment loop. Manual Run on this Mac uses the existing administrator bootstrap policy. |
| Renderer reload / another Jaz window | Restore the app's current verified session through the bounded memory handoff below, then verify normally; no disk unlock or separate registration for each window. |
| Whole app quits and restarts | Memory is lost. The existing nonsecret local launch preference invokes the same trusted bootstrap and obtains a fresh token for the same identity. No device key or token is copied to disk. A missing/rejected root grant, failed identity validation, or failed registration/auth check stops startup. |
| Fresh remote link or approved pairing | Use only the supplied/enrollment-issued device credential, verify it and retain it in memory. Remote HTTPS, approval and revocation checks remain. Local root credentials never authenticate a remote target. |
| Remote session ends | Without explicit remembering, the credential is unavailable after app exit. The server URL may remain in Recents; it grants no access. Reconnect requires a fresh authorized link/pairing or unlocking an existing saved credential. |
| Existing remembered remote | Its prior saved launch preference can retain the existing encrypted restore behavior. Keychain denial leaves it locked. A session token may come from fresh server authorization, never from ignoring the failed decrypt. |
| **Remember Connection** | A one-shot action in existing connection details, after connection, with compact **Session Only** / **Remembered** status. It encrypts the current verified token through the existing secure-storage boundary. Only a successful encrypted write establishes remembered access and a remote auto-reconnect preference. Failure leaves the working session and prior ciphertext intact: “Connected for this session. Couldn’t remember this connection.” No new connect wizard or required prompt. Local bootstrap makes remembering optional. |
| Disconnect / Forget | Disconnect clears the relevant session caches/tickets, stops its consumers and clears auto-connect intent; saved ciphertext stays intact. The existing explicit Forget action retains its separate meaning. Switching does not persist a session-only remote as the next auto-connect target. |

All fresh connections default to session-only retention. URL/history/local-start preferences contain no credentials. Saving the existing encrypted value is never a prerequisite for accepting a newly authorized session. Remembering never falls back to plaintext, `sessionStorage`, URL parameters, command-line arguments or diagnostic output.

Expose Remember Connection only where the secure-storage boundary exists. A plain browser with no encrypted writer supports memory-only access; it must never take the current writer's plaintext fallback. Existing ciphertext is touched only by an explicit encrypted remember/restore operation or the already separate Forget action.

## Necessary Desktop Detail

The current map is **per renderer**, not per application. Boards and the launcher have separate renderers, so merely moving `authTokens.set` before encryption would fail on reload or make windows re-register and invalidate one another.

Keep the existing renderer map for synchronous request headers. Add only a main-process memory handoff of verified origin/token snapshots for app-owned top-level windows, protected by `trustedWindow` and the exact backend origin. The main connection window publishes/replaces/clears snapshots; secondary windows hydrate and receive invalidation, never independently bootstrap or decrypt a saved token. First-launch secondary windows wait for the main connection result. Share device credentials only, not the root grant. Preview webviews, frames and external pages receive nothing. App exit drops the handoff; no persistence, new backend auth service, signing scheme or provider-auth change is proposed.

## Security And Behavior Tradeoffs Requiring Review

- Keychain gates **remembered access**; server approval/authentication gates **current access**. Verified bearer tokens already exist in process memory today. Session-only retention adds no encrypted-at-rest substitute and no new durable secret.
- Existing local root registration **reissues** the token: `saveApprovedDevice` stores one new token hash and cancels the old device's active requests. On cold local restart, old bearer tokens become invalid. Existing ciphertext remains byte-identical but may contain an obsolete local token. There is no reset command, device deletion or key regeneration, but preserving ciphertext does **not** preserve that old token's validity. If “no credential resets” also forbids this existing reissuance behavior, cold restart cannot use the current one-hash contract without a separate backend decision; implementation must stop there rather than claim otherwise.
- Existing root-authorized registration can also reapprove a revoked local identity. This administrator recovery power already exists; it must be accepted explicitly for automatic cold-start bootstrap. No weaker anonymous or remote recovery is proposed, and a live authorization rejection never silently invokes it.
- “Session Only” describes client retention. Current tokens have no new server-side quit expiry; a copied remote bearer remains subject to the existing rotation/revocation rules. Closing Jaz discards local copies, not server approval.

## Minimal Change Surface And Acceptance Gate

Reuse `api/client.ts`'s cache and encrypted writer, `connection.ts`'s registration/pairing/auth flow and saved preference, the already approved notification cache accessor, existing trusted IPC/preload boundaries for the memory handoff, and one existing connection-details action. Keep server/device schema, root-grant policy, identity keys, Keychain entries and provider auth unchanged. No runtime code has been changed for this proposal.

After independent approval, extend the preserved installed-byte/production fixtures to prove: denied Keychain methods are never called during fresh local/remote session activation; real authenticated API and stream use; same-identity local cold restart without old-token unlock; no secondary-window token churn; failed/revoked/mismatched grants remain blocked; cancellation cannot publish a late credential; old ciphertext stays byte-identical; explicit remember failure preserves access; saved remote decrypt denial stays locked. Account for token rotation and resource-ticket invalidation explicitly. The earlier registration-200 fixture alone is insufficient.

Source basis: `api/client.ts` (`authTokens`, `apiAuthToken`, `setApiAuthToken`), `connection.ts` (`startLocal`, `connectStoredToken`, `registerDevice`, pairing poll, `init`), main `backend.ts` / `permissions.ts` / board-window creation, and backend `server/auth.go`, `httpapi/devices/handler.go`, `deviceauth/service.go`, `deviceauth/access.go`, and `SaveApprovedDevice` SQL. Diagnosis, byte seals, screenshots and all earlier fixtures remain preserved in `runs/installed-device-registration/`.
