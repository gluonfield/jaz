// Build-time appearance defaults. These ship as a separate, swappable file —
// public/appearance-defaults.js — that sets window.__JAZ_APPEARANCE_DEFAULTS__
// synchronously before first paint. Edit that file (or replace it on a deployed
// static build, no rebuild needed) to brand an instance. Every field is optional:
// an empty object is the stock look, and these are only *defaults* — a user's own
// Settings → Appearance choices always layer on top. Read once at module load;
// lib/appearance.ts and lib/theme.ts fold it under the user's stored prefs, and
// the pre-paint script in index.html reads the same global.
// A per-mode color scheme override (all fields optional). Hex strings + a 0–100
// contrast; lib/appearanceScheme.ts validates and derives the full token set.
export interface ColorSchemeConfig {
  accent?: string
  background?: string
  foreground?: string
  contrast?: number
}

export interface AppearanceConfig {
  theme?: 'light' | 'dark' | 'system'
  uiFont?: string
  monoFont?: string
  fontScale?: number
  effects?: boolean
  wideLayout?: boolean
  inlineDiffs?: boolean
  inlineShellCommands?: boolean
  // Default color scheme. `preset` names a built-in (e.g. 'catppuccin'); the
  // per-mode blocks override individual colors on top of it (or of the default).
  scheme?: {
    preset?: string
    light?: ColorSchemeConfig
    dark?: ColorSchemeConfig
  }
}

declare global {
  interface Window {
    __JAZ_APPEARANCE_DEFAULTS__?: AppearanceConfig
  }
}

export function appearanceConfig(): AppearanceConfig {
  return window.__JAZ_APPEARANCE_DEFAULTS__ ?? {}
}
