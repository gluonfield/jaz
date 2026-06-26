import { resolvePreviewPatterns } from '../../../shared/preview'

// Build-time deploy defaults. These ship as a separate, swappable file —
// public/jaz-defaults.js — that sets window.__JAZ_DEFAULTS__ synchronously before
// first paint. Edit that file (or replace it on a deployed static build, no
// rebuild needed) to brand an instance. Every field is optional: an empty object
// is the stock look, and these are only *defaults* — a user's own Settings choices
// always layer on top. Read once at module load; lib/appearance.ts and lib/theme.ts
// fold it under the user's stored prefs, and the pre-paint script in index.html
// reads the same global.
// A per-mode color scheme override (all fields optional). Hex strings + a 0–100
// contrast; lib/appearanceScheme.ts validates and derives the full token set.
export interface ColorSchemeConfig {
  accent?: string
  background?: string
  foreground?: string
  contrast?: number
}

export interface ComposerConfig {
  hideModelPicker?: boolean
  hideProjectPicker?: boolean
}

// Per-agent new-thread defaults. The composer pre-selects these for a fresh
// thread; the user's per-thread pick wins, and an omitted field falls back to the
// Settings → Agents default.
export interface AgentDefaultConfig {
  model?: string
  reasoningEffort?: string
}

export interface JazDefaults {
  theme?: 'light' | 'dark' | 'system'
  uiFont?: string
  monoFont?: string
  fontScale?: number
  effects?: boolean
  wideLayout?: boolean
  inlineDiffs?: boolean
  inlineShellCommands?: boolean
  composer?: ComposerConfig
  // Default color scheme. `preset` names a built-in (e.g. 'catppuccin'); the
  // per-mode blocks override individual colors on top of it (or of the default).
  scheme?: {
    preset?: string
    light?: ColorSchemeConfig
    dark?: ColorSchemeConfig
  }
  // Inline web-preview cards: a URL in an assistant reply matching any of these
  // regex patterns gets an "Open preview" card below the message. Omit to use
  // the defaults (localhost + 127.0.0.1).
  previewPatterns?: string[]
  // New-thread defaults keyed by agent name (codex, claude, opencode, grok, …).
  agents?: Record<string, AgentDefaultConfig>
}

declare global {
  interface Window {
    __JAZ_DEFAULTS__?: JazDefaults
  }
}

export function jazDefaults(): JazDefaults {
  return window.__JAZ_DEFAULTS__ ?? {}
}

export function composerConfig(): ComposerConfig {
  return jazDefaults().composer ?? {}
}

export function previewPatterns(): string[] {
  return resolvePreviewPatterns(jazDefaults().previewPatterns)
}

export function agentDefault(agent: string): AgentDefaultConfig {
  return jazDefaults().agents?.[agent] ?? {}
}
