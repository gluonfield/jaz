# Jaz Chrome Extension

The signed-in Chrome bridge lives in a separate repository:

https://github.com/gluonfield/jaz-chrome-extension

The backend contract for that extension is versioned in
`docs/browser-extension-contract.json`. Backend tests verify Jaz's protocol,
action list, and semantic page-state JSON shape against that contract. The
extension repository carries the same contract file and validates its exported
protocol/action constants against it during `npm test` and `npm run build`.

