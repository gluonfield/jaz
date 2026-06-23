# Jaz Chrome Extension

The signed-in Chrome bridge lives in a separate repository:

https://github.com/gluonfield/jaz-chrome-extension

For a local desktop Jaz backend, the extension should use the default bridge URL
without any pasted credential:

```txt
ws://127.0.0.1:5299/v1/browser/extension
```

The backend accepts that unauthenticated path only for loopback requests whose
WebSocket Origin is a Chrome extension. Remote backend bridges still need an
authenticated `wss://` URL.

The backend contract for that extension is versioned in
`docs/browser-extension-contract.json`. Backend tests verify Jaz's protocol,
action list, and semantic page-state JSON shape against that contract. The
extension repository carries the same contract file and validates its exported
protocol/action constants against it during `npm test` and `npm run build`.
