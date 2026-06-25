// Build-time appearance defaults. These ship as a separate, swappable file —
// public/appearance-defaults.js — that sets window.__JAZ_APPEARANCE_DEFAULTS__
// synchronously before first paint. Edit that file (or replace it on a deployed
// static build, no rebuild needed) to brand an instance. Every field is optional:
// an empty object is the stock look, and these are only *defaults* — a user's own
// Settings → Appearance choices always layer on top. Read once at module load;
// lib/appearance.ts and lib/theme.ts fold it under the user's stored prefs, and
// the pre-paint script in index.html reads the same global.
export interface AppearanceConfig {
  theme?: 'light' | 'dark' | 'system'
  accent?: number
  uiFont?: string
  monoFont?: string
  fontScale?: number
  effects?: boolean
  wideLayout?: boolean
  inlineDiffs?: boolean
  inlineShellCommands?: boolean
}

declare global {
  interface Window {
    __JAZ_APPEARANCE_DEFAULTS__?: AppearanceConfig
  }
}

export function appearanceConfig(): AppearanceConfig {
  return window.__JAZ_APPEARANCE_DEFAULTS__ ?? {}
}
