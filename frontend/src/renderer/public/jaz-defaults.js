// Jaz appearance defaults — a separate, swappable config file (not part of the
// hashed bundle). Brand a build by uncommenting and setting any field below; an
// empty object is the stock look. These are only *defaults* — whatever a user
// picks in Settings → Appearance always overrides them. Loaded synchronously
// before first paint, so changes apply with no flash. On a static deploy
// (e.g. Cloudflare) you can replace just this file without rebuilding the app.
window.__JAZ_DEFAULTS__ = {
  // theme: 'system',              // 'light' | 'dark' | 'system'
  // uiFont: 'Inter',              // interface font family
  // monoFont: 'JetBrains Mono',   // code / diff font family
  // fontScale: 1,                 // 0.9 | 1 | 1.1 | 1.25
  // effects: true,                // decorative motion (composer glow, shimmer)
  // wideLayout: false,            // wider thread column
  // showModelIcons: false,        // ACP agent/model marks in the left sidebar
  // homeWordmark: 'jaz',          // name or image URL above the new-thread composer
  // invertHomeLogoInLightMode: false, // darkens white image logos in light mode
  // invertHomeLogoInDarkMode: false, // lightens black image logos in dark mode
  // inlineDiffs: false,           // expand agent file diffs in the transcript
  // inlineShellCommands: false,   // expand agent shell commands in the transcript

  // New-thread composer pickers (default false = shown). The agent picker
  // hides itself when only one agent is connected.
  // composer: {
  //   hideModelPicker: false,
  //   hideProjectPicker: false,
  // },

  // Default colour theme for light and dark (Settings → Appearance → Color).
  // Users can still change all of this in Settings.
  // color: {
  //   hue: 205,          // 0–360, the main colour
  //   colors: 2,         // 0 (graphite) to 3 linked colours
  //   harmony: 1,        // how they link; 2 colours: 0 complementary, 1 analogous;
  //                      // 3 colours: 0 analogous, 1 split complementary, 2 triadic
  //   saturation: 0.6,   // 0–1
  //   strength: 0.45,    // 0–1, how strongly they wash the window
  //   grain: 0,          // 0–1 film grain over the window
  // },

  // Inline web-preview cards. A URL in an assistant reply matching any of these
  // regex patterns gets an "Open preview" card below the message. Omit to use the
  // defaults (localhost + 127.0.0.1). Tested against the full URL, case-insensitive.
  // previewPatterns: ['localhost', '127\\.0\\.0\\.1', 'staging\\.example\\.com'],

  // Per-agent new-thread defaults (model + reasoning effort), keyed by agent name.
  // These pre-select the model/effort for a fresh thread; a choice the user makes
  // in that thread wins, and an omitted field falls back to Settings → Agents.
  // agents: {
  //   codex:    { model: 'gpt-6-astra', reasoningEffort: 'high' },
  //   claude:   { reasoningEffort: 'max' },
  //   opencode: { model: 'anthropic/claude-sonnet-4-6' },
  // },
}
