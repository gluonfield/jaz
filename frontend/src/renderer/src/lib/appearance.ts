import { useSyncExternalStore } from 'react'

// User-tunable appearance preferences, kept deliberately separate from the
// light/dark theme (lib/theme.ts). Same mechanics: persisted in localStorage,
// applied to the document root, mirrored into the pre-paint script in
// index.html, and synced across sibling Electron windows via the storage event.
// Defaults reproduce the stock look exactly, so an untouched install is
// byte-for-byte the current experience.
export interface AppearanceSettings {
  /** decorative motion: composer focus comet, shimmer dots, gradient wordmark, particle fields */
  effects: boolean
  /** whole-UI zoom factor; 1 is the stock size */
  fontScale: number
  /** interface font family name; '' keeps the default Inter stack */
  uiFont: string
  /** monospace font family name; '' keeps the default JetBrains Mono stack */
  monoFont: string
  /** render agent file edits as expanded red/green diffs in the transcript */
  inlineDiffs: boolean
  /** widen the whole thread column (messages, code, diffs, composer) */
  wideLayout: boolean
}

export const DEFAULTS: AppearanceSettings = {
  effects: true,
  fontScale: 1,
  uiFont: '',
  monoFont: '',
  inlineDiffs: false,
  wideLayout: false,
}

// Thread column + prose widths for the wide layout. Defaults (720px / 76ch) live
// in :root in globals.css; wide overrides both so prose, tool output and diffs
// all grow together. Keep in sync with the pre-paint script in index.html.
const WIDE_THREAD_MAX = '1040px'
const WIDE_PROSE_MAX = '1040px'

// Whole-UI zoom steps. The chrome is built largely with px sizes, so scaling the
// document root (CSS zoom) is the only thing that grows everything together.
export const FONT_SCALES = [0.9, 1, 1.1, 1.25] as const

const KEYS = {
  effects: 'jaz.appearance.effects',
  fontScale: 'jaz.appearance.fontScale',
  uiFont: 'jaz.appearance.uiFont',
  monoFont: 'jaz.appearance.monoFont',
  inlineDiffs: 'jaz.appearance.inlineDiffs',
  wideLayout: 'jaz.appearance.wideLayout',
} as const

const listeners = new Set<() => void>()

// These stacks must stay in sync with --font-sans/--font-mono in globals.css and
// with the inline pre-paint script in index.html.
export function uiFontStack(name: string): string {
  return `"${name}", 'Inter Variable', ui-sans-serif, system-ui, sans-serif`
}
export function monoFontStack(name: string): string {
  return `"${name}", 'JetBrains Mono Variable', ui-monospace, 'SF Mono', monospace`
}

function readStored(): AppearanceSettings {
  const scale = Number(localStorage.getItem(KEYS.fontScale))
  return {
    effects: localStorage.getItem(KEYS.effects) !== 'false',
    fontScale: Number.isFinite(scale) && scale > 0 ? scale : DEFAULTS.fontScale,
    uiFont: localStorage.getItem(KEYS.uiFont) ?? DEFAULTS.uiFont,
    monoFont: localStorage.getItem(KEYS.monoFont) ?? DEFAULTS.monoFont,
    inlineDiffs: localStorage.getItem(KEYS.inlineDiffs) === 'true',
    wideLayout: localStorage.getItem(KEYS.wideLayout) === 'true',
  }
}

let current: AppearanceSettings = readStored()

function apply(s: AppearanceSettings) {
  const root = document.documentElement
  // Effects: a single root class CSS keys off (mirrors prefers-reduced-motion);
  // JS-driven effects read the `effects` flag through useEffectsEnabled.
  root.classList.toggle('jaz-no-effects', !s.effects)
  // Font size: zoom the whole document so px chrome and rem prose grow together.
  if (s.fontScale && s.fontScale !== 1) root.style.setProperty('zoom', String(s.fontScale))
  else root.style.removeProperty('zoom')
  // Fonts: override the theme tokens inline on :root; clearing falls back to the
  // stack declared in @theme.
  const ui = s.uiFont.trim()
  if (ui) root.style.setProperty('--font-sans', uiFontStack(ui))
  else root.style.removeProperty('--font-sans')
  const mono = s.monoFont.trim()
  if (mono) root.style.setProperty('--font-mono', monoFontStack(mono))
  else root.style.removeProperty('--font-mono')
  // Content width: override the column + prose maxes when wide; clearing falls
  // back to the stock 720px / 76ch declared in :root.
  if (s.wideLayout) {
    root.style.setProperty('--thread-max', WIDE_THREAD_MAX)
    root.style.setProperty('--prose-max', WIDE_PROSE_MAX)
  } else {
    root.style.removeProperty('--thread-max')
    root.style.removeProperty('--prose-max')
  }
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
  current = { ...current, ...patch }
  if ('effects' in patch) persist('effects', String(current.effects), current.effects === DEFAULTS.effects)
  if ('fontScale' in patch)
    persist('fontScale', String(current.fontScale), current.fontScale === DEFAULTS.fontScale)
  if ('uiFont' in patch) persist('uiFont', current.uiFont, current.uiFont.trim() === '')
  if ('monoFont' in patch) persist('monoFont', current.monoFont, current.monoFont.trim() === '')
  if ('inlineDiffs' in patch)
    persist('inlineDiffs', String(current.inlineDiffs), current.inlineDiffs === DEFAULTS.inlineDiffs)
  if ('wideLayout' in patch)
    persist('wideLayout', String(current.wideLayout), current.wideLayout === DEFAULTS.wideLayout)
  apply(current)
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
