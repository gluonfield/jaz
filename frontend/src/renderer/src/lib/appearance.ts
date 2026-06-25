import { useSyncExternalStore } from 'react'
import { setServerDefaultTheme } from './theme'

// User-tunable appearance preferences, kept deliberately separate from the
// light/dark theme (lib/theme.ts). Same mechanics: persisted in localStorage,
// applied to the document root, mirrored into the pre-paint script in
// index.html, and synced across sibling Electron windows via the storage event.
// Defaults reproduce the stock look exactly, so an untouched install is
// byte-for-byte the current experience. The backend can supply deployment
// defaults (application.yaml jaz.ui, served on /health) that layer *under* a
// user's own choices — see applyServerUIDefaults.
export interface AppearanceSettings {
  /** decorative motion: composer focus comet, shimmer dots, gradient wordmark, particle fields */
  effects: boolean
  /** accent hue in oklch degrees; drives the whole --color-primary family via --accent-h */
  accent: number
  /** whole-UI zoom factor; 1 is the stock size */
  fontScale: number
  /** interface font family name; '' keeps the default Inter stack */
  uiFont: string
  /** monospace font family name; '' keeps the default JetBrains Mono stack */
  monoFont: string
  /** render agent file edits as expanded red/green diffs in the transcript */
  inlineDiffs: boolean
  /** render the agent's shell commands (command + output) inline in the transcript */
  inlineShellCommands: boolean
  /** widen the whole thread column (messages, code, diffs, composer) */
  wideLayout: boolean
}

// Stock accent hue (cobalt). Matches the --accent-h fallback in globals.css; an
// untouched install never writes the key, so the CSS default rules.
export const DEFAULT_ACCENT_HUE = 262

export const DEFAULTS: AppearanceSettings = {
  effects: true,
  accent: DEFAULT_ACCENT_HUE,
  fontScale: 1,
  uiFont: '',
  monoFont: '',
  inlineDiffs: false,
  inlineShellCommands: false,
  wideLayout: false,
}

// Whole-UI zoom steps. The chrome is built largely with px sizes, so scaling the
// document root (CSS zoom) is the only thing that grows everything together.
export const FONT_SCALES = [0.9, 1, 1.1, 1.25] as const

// Curated accent palette. Each preset is just an oklch hue: the primary family
// in globals.css carries its own lightness/chroma per token and per theme, so a
// hue is all it takes to recolour the accent everywhere. Cobalt is the default.
export interface AccentPreset {
  id: string
  label: string
  hue: number
}

export const ACCENT_PRESETS: readonly AccentPreset[] = [
  { id: 'cobalt', label: 'Cobalt', hue: DEFAULT_ACCENT_HUE },
  { id: 'azure', label: 'Azure', hue: 230 },
  { id: 'teal', label: 'Teal', hue: 195 },
  { id: 'green', label: 'Green', hue: 150 },
  { id: 'amber', label: 'Amber', hue: 75 },
  { id: 'rose', label: 'Rose', hue: 12 },
  { id: 'magenta', label: 'Magenta', hue: 340 },
  { id: 'violet', label: 'Violet', hue: 300 },
] as const

const KEYS = {
  effects: 'jaz.appearance.effects',
  accent: 'jaz.appearance.accent',
  fontScale: 'jaz.appearance.fontScale',
  uiFont: 'jaz.appearance.uiFont',
  monoFont: 'jaz.appearance.monoFont',
  inlineDiffs: 'jaz.appearance.inlineDiffs',
  inlineShellCommands: 'jaz.appearance.inlineShellCommands',
  wideLayout: 'jaz.appearance.wideLayout',
} as const

const listeners = new Set<() => void>()

export function normalizeFontScale(value: unknown): AppearanceSettings['fontScale'] {
  const scale = typeof value === 'number' ? value : Number(value)
  return FONT_SCALES.find((candidate) => Math.abs(candidate - scale) < 0.001) ?? DEFAULTS.fontScale
}

export function normalizeAccent(value: unknown): AppearanceSettings['accent'] {
  const hue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(hue) && hue >= 0 && hue < 360 ? hue : DEFAULT_ACCENT_HUE
}

function fontName(value: string | null): string {
  return value?.trim() ?? ''
}

function cssFontName(name: string): string {
  return `"${name.replace(/["\\]/g, '')}"`
}

// Operator defaults from the backend (/health `ui`), in the wire shape. Every
// field is optional ("unset"); `theme` is consumed by lib/theme.ts, the rest map
// below. Cached in localStorage so the pre-paint script in index.html can read
// it on the next launch without waiting for the connection.
export interface ServerUIDefaults {
  theme?: string
  accent?: number
  ui_font?: string
  mono_font?: string
  font_scale?: number
  effects?: boolean
  wide_layout?: boolean
  inline_diffs?: boolean
  inline_shell_commands?: boolean
}

const SERVER_DEFAULTS_KEY = 'jaz.serverDefaults'

function readCachedServerUI(): ServerUIDefaults {
  try {
    const raw = localStorage.getItem(SERVER_DEFAULTS_KEY)
    return raw ? (JSON.parse(raw) as ServerUIDefaults) : {}
  } catch {
    return {}
  }
}

function toServerPartial(ui: ServerUIDefaults): Partial<AppearanceSettings> {
  const p: Partial<AppearanceSettings> = {}
  if (typeof ui.accent === 'number') p.accent = normalizeAccent(ui.accent)
  if (typeof ui.font_scale === 'number') p.fontScale = normalizeFontScale(ui.font_scale)
  if (typeof ui.ui_font === 'string') p.uiFont = fontName(ui.ui_font)
  if (typeof ui.mono_font === 'string') p.monoFont = fontName(ui.mono_font)
  if (typeof ui.effects === 'boolean') p.effects = ui.effects
  if (typeof ui.wide_layout === 'boolean') p.wideLayout = ui.wide_layout
  if (typeof ui.inline_diffs === 'boolean') p.inlineDiffs = ui.inline_diffs
  if (typeof ui.inline_shell_commands === 'boolean') p.inlineShellCommands = ui.inline_shell_commands
  return p
}

// Server defaults overlay the hardcoded DEFAULTS to form the base a client falls
// back to for any field the user hasn't explicitly set.
let serverDefaults: Partial<AppearanceSettings> = toServerPartial(readCachedServerUI())

function baseDefaults(): AppearanceSettings {
  return { ...DEFAULTS, ...serverDefaults }
}

function readStored(): AppearanceSettings {
  const base = baseDefaults()
  const bool = (key: string, fallback: boolean): boolean => {
    const v = localStorage.getItem(key)
    return v === null ? fallback : v === 'true'
  }
  const accentRaw = localStorage.getItem(KEYS.accent)
  const scaleRaw = localStorage.getItem(KEYS.fontScale)
  const uiFontRaw = localStorage.getItem(KEYS.uiFont)
  const monoFontRaw = localStorage.getItem(KEYS.monoFont)
  return {
    effects: bool(KEYS.effects, base.effects),
    accent: accentRaw === null ? base.accent : normalizeAccent(accentRaw),
    fontScale: scaleRaw === null ? base.fontScale : normalizeFontScale(scaleRaw),
    uiFont: uiFontRaw === null ? base.uiFont : fontName(uiFontRaw),
    monoFont: monoFontRaw === null ? base.monoFont : fontName(monoFontRaw),
    inlineDiffs: bool(KEYS.inlineDiffs, base.inlineDiffs),
    inlineShellCommands: bool(KEYS.inlineShellCommands, base.inlineShellCommands),
    wideLayout: bool(KEYS.wideLayout, base.wideLayout),
  }
}

let current: AppearanceSettings = readStored()

function apply(s: AppearanceSettings) {
  const root = document.documentElement
  // Effects: a single root class CSS keys off (mirrors prefers-reduced-motion);
  // JS-driven effects read the `effects` flag through useEffectsEnabled.
  root.classList.toggle('jaz-no-effects', !s.effects)
  // Accent: a single hue feeds the whole --color-primary family in globals.css.
  if (s.accent !== DEFAULT_ACCENT_HUE) root.style.setProperty('--accent-h', String(s.accent))
  else root.style.removeProperty('--accent-h')
  // Font size: zoom the whole document so px chrome and rem prose grow together.
  if (s.fontScale && s.fontScale !== 1) root.style.setProperty('zoom', String(s.fontScale))
  else root.style.removeProperty('zoom')
  // Fonts: override only the first family; globals.css owns the fallback stacks.
  const ui = fontName(s.uiFont)
  if (ui) root.style.setProperty('--jaz-ui-font', cssFontName(ui))
  else root.style.removeProperty('--jaz-ui-font')
  const mono = fontName(s.monoFont)
  if (mono) root.style.setProperty('--jaz-mono-font', cssFontName(mono))
  else root.style.removeProperty('--jaz-mono-font')
  root.classList.toggle('jaz-wide-layout', s.wideLayout)
}

function persist(key: keyof AppearanceSettings, value: string, isDefault: boolean) {
  if (isDefault) localStorage.removeItem(KEYS[key])
  else localStorage.setItem(KEYS[key], value)
}

function notify() {
  for (const l of listeners) l()
}

export function getAppearance(): AppearanceSettings {
  return current
}

export function setAppearance(patch: Partial<AppearanceSettings>) {
  current = {
    ...current,
    ...patch,
    accent: 'accent' in patch ? normalizeAccent(patch.accent) : current.accent,
    fontScale: 'fontScale' in patch ? normalizeFontScale(patch.fontScale) : current.fontScale,
    uiFont: 'uiFont' in patch ? fontName(patch.uiFont ?? '') : current.uiFont,
    monoFont: 'monoFont' in patch ? fontName(patch.monoFont ?? '') : current.monoFont,
  }
  // A key is stored only when it differs from the base (server defaults over
  // hardcoded); choosing the base value clears the override so it keeps tracking
  // the deployment default.
  const base = baseDefaults()
  if ('effects' in patch) persist('effects', String(current.effects), current.effects === base.effects)
  if ('accent' in patch) persist('accent', String(current.accent), current.accent === base.accent)
  if ('fontScale' in patch)
    persist('fontScale', String(current.fontScale), current.fontScale === base.fontScale)
  if ('uiFont' in patch) persist('uiFont', current.uiFont, current.uiFont === base.uiFont)
  if ('monoFont' in patch) persist('monoFont', current.monoFont, current.monoFont === base.monoFont)
  if ('inlineDiffs' in patch)
    persist('inlineDiffs', String(current.inlineDiffs), current.inlineDiffs === base.inlineDiffs)
  if ('inlineShellCommands' in patch)
    persist(
      'inlineShellCommands',
      String(current.inlineShellCommands),
      current.inlineShellCommands === base.inlineShellCommands,
    )
  if ('wideLayout' in patch)
    persist('wideLayout', String(current.wideLayout), current.wideLayout === base.wideLayout)
  apply(current)
  notify()
}

// Adopt deployment defaults from the connected backend (/health `ui`). They
// become the base for any field the user hasn't pinned; explicit choices in
// localStorage still win. Caches the raw shape for the next launch's pre-paint
// script and hands the theme to lib/theme.ts. No-ops when unchanged.
let lastServerDefaults = JSON.stringify(readCachedServerUI())

export function applyServerUIDefaults(ui?: ServerUIDefaults): void {
  const next = ui ?? {}
  const serialized = JSON.stringify(next)
  if (serialized === lastServerDefaults) return
  lastServerDefaults = serialized
  if (Object.keys(next).length === 0) localStorage.removeItem(SERVER_DEFAULTS_KEY)
  else localStorage.setItem(SERVER_DEFAULTS_KEY, serialized)
  serverDefaults = toServerPartial(next)
  current = readStored()
  apply(current)
  setServerDefaultTheme(next.theme)
  notify()
}

function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

// Keep sibling Electron windows (detached boards, popouts) in step when the
// settings panel writes a preference from another window.
window.addEventListener('storage', (event) => {
  if (event.storageArea !== localStorage || !event.key?.startsWith('jaz.appearance.')) return
  current = readStored()
  apply(current)
  notify()
})

// Apply once at import. The inline FOUC script in index.html already set most of
// this for first paint; this keeps the in-memory state and the DOM aligned.
apply(current)

export function useAppearance() {
  const settings = useSyncExternalStore(subscribe, getAppearance)
  return { settings, setAppearance }
}

// Decorative motion is on unless the user turned effects off. Components combine
// this with the OS prefers-reduced-motion signal at their call site.
export function useEffectsEnabled(): boolean {
  return useSyncExternalStore(subscribe, () => current.effects)
}

export function useInlineDiffs(): boolean {
  return useSyncExternalStore(subscribe, () => current.inlineDiffs)
}

export function useInlineShellCommands(): boolean {
  return useSyncExternalStore(subscribe, () => current.inlineShellCommands)
}
