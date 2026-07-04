import { useSyncExternalStore } from 'react'
import { type JazDefaults, jazDefaults } from './jazDefaults'

// User-tunable appearance preferences, kept deliberately separate from the
// light/dark theme (lib/theme.ts). Same mechanics: persisted in localStorage,
// applied to the document root, mirrored into the pre-paint script in
// index.html, and synced across sibling Electron windows via the storage event.
// Build-time defaults from the appearance defaults file (see jazDefaults.ts)
// layer *under* a user's own choices; with no config, an untouched install is
// byte-for-byte the stock look.
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
  /** show ACP agent/model marks in the left sidebar */
  showModelIcons: boolean
}

export const DEFAULTS: AppearanceSettings = {
  effects: true,
  fontScale: 1,
  uiFont: '',
  monoFont: '',
  inlineDiffs: false,
  inlineShellCommands: false,
  wideLayout: false,
  showModelIcons: true,
}

// Whole-UI zoom steps. The chrome is built largely with px sizes, so scaling the
// document root (CSS zoom) is the only thing that grows everything together.
export const FONT_SCALES = [0.9, 1, 1.1, 1.25] as const

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

// Each field's persistence contract in one place: how it reads from the
// build-time config, decodes from / encodes to localStorage, and normalizes a
// programmatically-set value. apply() owns the (heterogeneous) DOM side. Adding
// a field is one entry here plus its DEFAULTS / JazDefaults / FOUC counterparts.
interface Field<T> {
  storageKey: string
  fromConfig(cfg: JazDefaults): T | undefined
  decode(raw: string): T
  encode(value: T): string
  normalize(value: T): T
}

const boolField = (
  storageKey: string,
  pick: (cfg: JazDefaults) => boolean | undefined,
): Field<boolean> => ({
  storageKey,
  fromConfig: pick,
  decode: (raw) => raw === 'true',
  encode: String,
  normalize: (v) => v,
})

const fontField = (
  storageKey: string,
  pick: (cfg: JazDefaults) => string | undefined,
): Field<string> => ({
  storageKey,
  fromConfig: (cfg) => {
    const v = pick(cfg)
    return typeof v === 'string' ? fontName(v) : undefined
  },
  decode: fontName,
  encode: (v) => v,
  normalize: fontName,
})

const numberField = (
  storageKey: string,
  pick: (cfg: JazDefaults) => unknown,
  normalize: (value: unknown) => number,
): Field<number> => ({
  storageKey,
  fromConfig: (cfg) => {
    const v = pick(cfg)
    return typeof v === 'number' ? normalize(v) : undefined
  },
  decode: normalize,
  encode: String,
  normalize,
})

const FIELDS: { [K in keyof AppearanceSettings]: Field<AppearanceSettings[K]> } = {
  effects: boolField('jaz.appearance.effects', (c) => c.effects),
  fontScale: numberField('jaz.appearance.fontScale', (c) => c.fontScale, normalizeFontScale),
  uiFont: fontField('jaz.appearance.uiFont', (c) => c.uiFont),
  monoFont: fontField('jaz.appearance.monoFont', (c) => c.monoFont),
  inlineDiffs: boolField('jaz.appearance.inlineDiffs', (c) => c.inlineDiffs),
  inlineShellCommands: boolField('jaz.appearance.inlineShellCommands', (c) => c.inlineShellCommands),
  wideLayout: boolField('jaz.appearance.wideLayout', (c) => c.wideLayout),
  showModelIcons: boolField('jaz.appearance.showModelIcons', (c) => c.showModelIcons),
}

const FIELD_KEYS = Object.keys(FIELDS) as (keyof AppearanceSettings)[]

// The base a client falls back to for any field the user hasn't set: build-time
// config (jaz-defaults.js) over the hardcoded DEFAULTS. Static — the
// config is fixed at build time.
const BASE_DEFAULTS: AppearanceSettings = (() => {
  const cfg = jazDefaults()
  const base = { ...DEFAULTS }
  const seed = <K extends keyof AppearanceSettings>(key: K) => {
    const v = FIELDS[key].fromConfig(cfg)
    if (v !== undefined) base[key] = v
  }
  for (const key of FIELD_KEYS) seed(key)
  return base
})()

function readStored(): AppearanceSettings {
  const out = { ...BASE_DEFAULTS }
  const load = <K extends keyof AppearanceSettings>(key: K) => {
    const raw = localStorage.getItem(FIELDS[key].storageKey)
    if (raw !== null) out[key] = FIELDS[key].decode(raw)
  }
  for (const key of FIELD_KEYS) load(key)
  return out
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

function notify() {
  for (const l of listeners) l()
}

export function getAppearance(): AppearanceSettings {
  return current
}

export function setAppearance(patch: Partial<AppearanceSettings>) {
  const next = { ...current }
  // A settings-panel write is an explicit user choice. Deploy defaults only
  // apply to fields the user has never set.
  const store = <K extends keyof AppearanceSettings>(key: K, value: AppearanceSettings[K]) => {
    const field = FIELDS[key]
    const normalized = field.normalize(value)
    next[key] = normalized
    localStorage.setItem(field.storageKey, field.encode(normalized))
  }
  for (const key of FIELD_KEYS) {
    const value = patch[key]
    if (value !== undefined) store(key, value)
  }
  current = next
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
  if (event.storageArea && event.storageArea !== localStorage) return
  if (event.key !== null && !event.key.startsWith('jaz.appearance.')) return
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

export function useShowModelIcons(): boolean {
  return useSyncExternalStore(subscribe, () => current.showModelIcons)
}
