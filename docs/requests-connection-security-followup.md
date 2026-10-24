# Connection security follow-up

- [x] Review the current source and distinguish the previous findings from active behavior.
- [x] Check the localhost listener and tunnel authentication without changing live settings/devices.
- [x] Reproduce remaining pairing and credential-storage defects using production components in isolation.
- [x] Run relevant Go 1.26 security regressions and the device HTTP tests.
- [x] Apply the strict review pass and deliver the current report.

Report: `security-review-connections-current-2026-10-24.html`.
Source: `133d2517`. Four Medium findings, one conditional on manual MCP configuration; approved devices have broad administrative authority.
Augustinas subsequently requested: "fix issues". Implementation is authorised.

- [x] Invalidate older pairing grants atomically on revocation and credential replacement.
- [x] Keep proposed profiles separate from approved device records until exchange.
- [x] Bound anonymous pairing admission, pending requests and retained records.
- [x] Encrypt manual MCP secrets, including existing records, while preserving edit and runtime behavior.
- [x] Limit device-management endpoints to the host owner's credential and preserve desktop/phone flows.
- [x] Verify regressions, migration, client behavior and strict code quality.
- [ ] Commit, integrate and verify the active runtime.

Implementation: migration 79 stores the proposed profile on its pairing. Admission is atomic: five requests per minute, 32 unexpired grants, 24-hour retention. Revocation, replacement and exchange consume older authorizations. Manual MCP secrets use the existing AES-GCM boundary; HTTP reads return presence only, and omitted values preserve credentials for the same endpoint. Explicit empty values remove them. Device management uses the host root credential; ordinary approved clients retain normal chat/agent access. The local IPC entrypoint now checks the existing trusted-window boundary.

Verification: full Go 1.26 suite and vet; focused race regressions; 366 frontend tests/typecheck/lint and desktop/web builds; normal-path hidden Electron settings journeys at desktop and phone widths. An isolated real MCP server verifies bearer/header authentication before and after reopening encrypted storage. A private live-database clone migrates 77→79 with identical device credentials and all six OAuth envelopes/metadata, six decryptions, no outstanding old grants and a clean integrity check. The private pre-migration backup remains untouched. The snapshot includes already-integrated migration 78 as well.

Strict review moved HTTP secret parsing/redaction into `httpapi/mcp`, retained atomic storage ownership and removed duplicated joined-row mapping. It also caught and corrected saved-secret normalization, cross-endpoint credential preservation and the unguarded host-credential IPC entrypoint. No provider model, prompt, tool, compaction or authentication code changed; no native subscription experiments were launched. Phone/extension installed versions are unchanged; physical phone acceptance is unverified.

Review conclusions: pending identity data belongs to a pairing request; credential replacement/revocation must invalidate older grants atomically; secret values belong to encrypted storage. Prefer these owning-layer corrections over UI checks or endpoint-specific flags. The report distinguishes verified behavior, prerequisites, dormant configuration paths, intended authority and untested edge/device behavior.

| Before | After |
| --- | --- |
| Every approved client saw Add, address editing, approval and revocation controls. | Host-only controls follow the server's management capability; remote clients see the device list and a concise ownership message. |
| Saved MCP keys and literal headers were returned as form values. | Forms show saved-value placeholders, preserve untouched secrets and allow explicit token removal. |
