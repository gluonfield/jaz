# Jaz Chrome Extension

The signed-in Chrome bridge lives in a separate TypeScript MV3 side-panel
extension repository:

https://github.com/gluonfield/jaz-chrome-extension

For a local desktop Jaz backend, the extension should use the default bridge URL
without any pasted credential:

```txt
ws://127.0.0.1:5299/v1/browser/extension
```

The backend accepts that unauthenticated path only for loopback requests whose
WebSocket Origin is a Chrome extension. Remote backend bridges still need an
authenticated `wss://` URL.

When Browser is enabled in Jaz settings, ordinary agent sessions receive typed
extension-backed browser tools directly. Jaz does not spawn a separate coding
agent to interpret or relay browser actions.

The normal interaction path is `browser_tabs` or `browser_navigate`, followed by
`browser_read_page` or `browser_find`, then a ref-only action such as
`browser_click` or `browser_form_input`. Page refs carry a page revision and are
rejected after they become stale. Each new `browser_read_page` or `browser_find`
result replaces the prior ref set. CSS selectors are not part of the MCP tool
contract. Action tools return their own result without silently performing a
second page read; call `browser_read_page` explicitly when the next step depends
on updated state.

The extension's `browser_key` action dispatches DOM keyboard events. It can
activate page-level keyboard handlers, but Chrome does not apply every trusted
native default (for example, Tab focus traversal); use ref-based click and
form-input actions for deterministic control.

Protocol v2 requires Jaz Browser Bridge 0.2.0 or newer; reload the updated
extension after upgrading Jaz.

The backend contract for that extension is versioned in
`docs/browser-extension-contract.json`. Backend tests verify Jaz's protocol,
action list, and semantic page-state JSON shape against that contract. The
extension repository carries the same contract file and validates its exported
protocol/action constants against it during `npm test` and `npm run build`.
