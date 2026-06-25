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
  /** render the agent's shell commands (command + output) inline in the transcript */
  inlineShellCommands: boolean
  /** widen the whole thread column (messages, code, diffs, composer) */
  wideLayout: boolean
}

export const DEFAULTS: AppearanceSettings = {
  effects: true,
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

const KEYS = {
  effects: 'jaz.appearance.effects',
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

function fontName(value: string | null): string {
  return value?.trim() ?? ''
}

function cssFontName(name: string): string {
  return `"${name.replace(/["\\]/g, '')}"`
}

function readStored(): AppearanceSettings {
  return {
    effects: localStorage.getItem(KEYS.effects) !== 'false',
    fontScale: normalizeFontScale(localStorage.getItem(KEYS.fontScale)),
    uiFont: fontName(localStorage.getItem(KEYS.uiFont)),
    monoFont: fontName(localStorage.getItem(KEYS.monoFont)),
    inlineDiffs: localStorage.getItem(KEYS.inlineDiffs) === 'true',
    inlineShellCommands: localStorage.getItem(KEYS.inlineShellCommands) === 'true',
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
    fontScale: 'fontScale' in patch ? normalizeFontScale(patch.fontScale) : current.fontScale,
    uiFont: 'uiFont' in patch ? fontName(patch.uiFont ?? '') : current.uiFont,
    monoFont: 'monoFont' in patch ? fontName(patch.monoFont ?? '') : current.monoFont,
  }
  if ('effects' in patch) persist('effects', String(current.effects), current.effects === DEFAULTS.effects)
  if ('fontScale' in patch)
    persist('fontScale', String(current.fontScale), current.fontScale === DEFAULTS.fontScale)
  if ('uiFont' in patch) persist('uiFont', current.uiFont, current.uiFont.trim() === '')
  if ('monoFont' in patch) persist('monoFont', current.monoFont, current.monoFont.trim() === '')
  if ('inlineDiffs' in patch)
    persist('inlineDiffs', String(current.inlineDiffs), current.inlineDiffs === DEFAULTS.inlineDiffs)
  if ('inlineShellCommands' in patch)
    persist(
      'inlineShellCommands',
      String(current.inlineShellCommands),
      current.inlineShellCommands === DEFAULTS.inlineShellCommands,
    )
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

export function useInlineShellCommands(): boolean {
  return useSyncExternalStore(subscribe, () => current.inlineShellCommands)
}
