# Connection security follow-up

- [x] Review the current source and distinguish the previous findings from active behavior.
- [x] Check the localhost listener and tunnel authentication without changing live settings/devices.
- [x] Reproduce remaining pairing and credential-storage defects using production components in isolation.
- [x] Run relevant Go 1.26 security regressions and the device HTTP tests.
- [x] Apply the strict review pass and deliver the current report.

Report: `security-review-connections-current-2026-10-24.html`.
Source: `133d2517`. Four Medium findings, one conditional on manual MCP configuration; approved devices have broad administrative authority.
Scope: assessment only; no production implementation or live configuration changes.

Review conclusions: pending identity data belongs to a pairing request; credential replacement/revocation must invalidate older grants atomically; secret values belong to encrypted storage. Prefer these owning-layer corrections over UI checks or endpoint-specific flags. The report distinguishes verified behavior, prerequisites, dormant configuration paths, intended authority and untested edge/device behavior.
