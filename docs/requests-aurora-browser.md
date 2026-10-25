# Side Browser Only

- [x] Diagnose and fix Aurora’s authenticated browser connection.
- [x] Remove Background Chromium and Extension from UI and backend.
- [x] Remove backend-mode fields, extension route and external-tab claiming.
- [x] Verify agent activity reveals its owning browser.
- [x] Integrate the final correction locally (code `0a8dc58d`).
- [x] Complete the requested thermo-nuclear review and fix its findings.

Jaz has one browser per conversation. Agent navigation, page reads and scripts reveal that conversation’s side panel. Activity in another conversation leaves the current chat alone and reveals the retained browser when its owner is opened.

Browser settings contain only enablement and profile import. The backend stores/returns only enablement; old mode fields are ignored when loading saved records and rejected in new API updates. The extension bridge, route, origin exception, tab-claim tool and obsolete documentation are deleted. Shared script-result decoding remains with the desktop transport.

Verification: Go 1.26.3 tests/vet, 376 frontend tests, typecheck/lint; hidden Electron authenticated MCP reading/input; desktop/phone settings toggles; first navigation, reopening after reads/scripts, and background activity on return. The focused visibility test runs through production BrowserWorkspace, panel and browser components in the normal browser smoke path.

Evidence: `runs/aurora-browser/side-only-*` and `visibility-closed-owner.log`; screenshots in `runs/browser-smoke-OOYwpz/` and `runs/browser-smoke-hJhu1z/`. The older broad panel suite retains its unrelated layout failure; its assertions and product layout were preserved. Direct strict review kept one settings gate and removed mode routing and wrappers. No live server restart, push or release.

Review corrections: deleted the unreachable raw-CDP navigation path and its polling helpers (64 lines). Browser updates require an explicit boolean and save it directly; empty/null requests are rejected and malformed saved records can be replaced. Both HTTP regressions fail on the previous implementation (`review-old-settings.log`). Hidden settings verification now uses the production HTTP handler instead of a simulated switch state; browser fixtures restore credentials and close their socket on failure. Script-result decoding remains a transport boundary preserving the 32 MiB encoded-image limit.

Review validation: affected Go race tests/vet, focused server security checks, 13 frontend tests/typecheck/lint, hidden Electron settings and visibility checks pass. Latest screenshots: `runs/browser-smoke-AROGs8/` and `runs/browser-smoke-isJAKd/`. Initial fixture PUT preflight failure is fixed. The extra broad server race run was stopped during unrelated SQLite migration setup in a Kimi settings test; its trace remains in `review-go.log`, not counted as a pass. No unresolved findings in the browser change scope.
