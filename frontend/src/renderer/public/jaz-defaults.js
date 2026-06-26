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
  // inlineDiffs: false,           // expand agent file diffs in the transcript
  // inlineShellCommands: false,   // expand agent shell commands in the transcript

  // New-thread composer pickers (default false = shown). The agent picker
  // hides itself when only one agent is connected.
  // composer: {
  //   hideModelPicker: false,
  //   hideProjectPicker: false,
  // },

  // Default color scheme for light + dark. `preset` names a built-in (jaz,
  // catppuccin, github, gruvbox, rose-pine, solarized, nord, tokyo-night,
  // everforest, one); the per-mode blocks override individual colors on top.
  // contrast is 0–100. Users can still change all of this in Settings.
  // scheme: {
  //   preset: 'catppuccin',
  //   light: { accent: '#8839ef', background: '#eff1f5', foreground: '#4c4f69', contrast: 45 },
  //   dark:  { accent: '#cba6f7', background: '#1e1e2e', foreground: '#cdd6f4', contrast: 55 },
  // },

  // Inline web-preview cards. A URL in an assistant reply matching any of these
  // regex patterns gets an "Open preview" card below the message. Omit to use the
  // defaults (localhost + 127.0.0.1). Tested against the full URL, case-insensitive.
  // previewPatterns: ['localhost', '127\\.0\\.0\\.1', 'staging\\.example\\.com'],

  // Per-agent new-thread defaults (model + reasoning effort), keyed by agent name.
  // These pre-select the model/effort for a fresh thread; a choice the user makes
  // in that thread wins, and an omitted field falls back to Settings → Agents.
  // agents: {
  //   codex:    { model: 'gpt-5.5', reasoningEffort: 'high' },
  //   claude:   { reasoningEffort: 'max' },
  //   opencode: { model: 'anthropic/claude-sonnet-4-6' },
  // },
}
