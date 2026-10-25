# Aurora Browser Connection

- [x] Diagnose thread `20261025T132858-17787445` from native results and hidden Chromium.
- [x] Fix authenticated browser, Computer Use and Terminal WebSocket negotiation.
- [x] Remove Background Chromium from settings, API and launcher.
- [x] Verify and review the implementation.
- [ ] Integrate locally.

The client now offers `jaz` alongside its credential. Shared upgrade policy selects only `jaz`, keeping credentials out of responses. Chromium accepts the connection and the browser registers under its conversation. The previous credential-only/no-selected-protocol handshake was rejected by Chromium; Gorilla's Go client had accepted it.

Background Chromium and its fallback routing are deleted. Saved `managed` selections resolve to the side browser; new requests for that mode are rejected. Current choices are Jaz side browser and Extension.

Verification: Go 1.26.3 full tests/vet; 376 frontend tests, typecheck/lint; hidden Electron authenticated MCP navigation/reading/trusted input and invalid-credential rejection; desktop/phone settings switching and visual inspection. Removing protocol negotiation through a Go overlay makes the real Electron regression fail. Direct strict review removed the obsolete launcher and redundant constructor, retaining shared page actions.

Evidence: `runs/aurora-browser/`, with final renders in `runs/browser-smoke-djNUh3/`. No provider metadata, prompts, native auth or runtime versions changed. No live restart or release.

A broader side-panel smoke stops at the untouched tab-layout assertion before browser work (`side-panel.log`); relevant production panel/control sources equal main. The focused authenticated MCP/Chromium and settings test passes (`mcp-read.log`). Integration retains main’s independently merged shared HTTP upgrader/public `jaz` protocol.
