// Jaz appearance defaults — a separate, swappable config file (not part of the
// hashed bundle). Brand a build by uncommenting and setting any field below; an
// empty object is the stock look. These are only *defaults* — whatever a user
// picks in Settings → Appearance always overrides them. Loaded synchronously
// before first paint, so changes apply with no flash. On a static deploy
// (e.g. Cloudflare) you can replace just this file without rebuilding the app.
window.__JAZ_APPEARANCE_DEFAULTS__ = {
  // theme: 'system',              // 'light' | 'dark' | 'system'
  // uiFont: 'Inter',              // interface font family
  // monoFont: 'JetBrains Mono',   // code / diff font family
  // fontScale: 1,                 // 0.9 | 1 | 1.1 | 1.25
  // effects: true,                // decorative motion (composer glow, shimmer)
  // wideLayout: false,            // wider thread column
  // inlineDiffs: false,           // expand agent file diffs in the transcript
  // inlineShellCommands: false,   // expand agent shell commands in the transcript
}
