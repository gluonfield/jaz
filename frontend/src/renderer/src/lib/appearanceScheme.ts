import { useSyncExternalStore } from 'react'
import { jazDefaults, type ColorSchemeConfig } from './jazDefaults'

// A color scheme is four user-facing inputs per light/dark mode; every other
// --color-* token (surfaces, ink ramp, borders, primary family) is *derived*
// from these via color-mix so a scheme stays internally coherent. Mirrors the
// Codex appearance model: accent / background / foreground / contrast.
export interface ColorScheme {
  accent: string
  background: string
  foreground: string
  /** 0–100: how far surfaces/ink step away from the background */
  contrast: number
}

export interface ModeSchemes {
  light: ColorScheme
  dark: ColorScheme
}

export interface ThemePreset {
  id: string
  label: string
  light: ColorScheme
  dark: ColorScheme
}

const scheme = (accent: string, background: string, foreground: string, contrast: number): ColorScheme => ({
  accent,
  background,
  foreground,
  contrast,
})

// Named example schemes. Each carries a light and dark variant; the per-mode
// preset pickers apply the matching variant to that mode only. The first entry
// ("Jaz") is the default — equal to it, schemeCss emits no override, so the
// stock globals.css tokens rule. "Codex" is the exact stock chrome theme from
// ChatGPT.app (surface/ink/accent/contrast); paste a Copy theme string for any
// custom Codex combination.
export const THEME_PRESETS: readonly ThemePreset[] = [
  { id: 'jaz', label: 'Jaz', light: scheme('#3b5bdb', '#eef0f5', '#2a2e3a', 45), dark: scheme('#8aa6ff', '#1b1d24', '#edf0f5', 55) },
  // Exact defaults reverse-engineered from Codex desktop chromeTheme he.light/he.dark.
  { id: 'codex', label: 'Codex', light: scheme('#339cff', '#ffffff', '#1a1c1f', 45), dark: scheme('#339cff', '#181818', '#ffffff', 60) },
  // Explicit chrome seeds from Codex code themes (ChatGPT.app asar).
  { id: 'ayu', label: 'Ayu', light: scheme('#f29718', '#fcfcfc', '#5c6166', 45), dark: scheme('#e6b450', '#10141c', '#bfbdb6', 60) },
  { id: 'linear', label: 'Linear', light: scheme('#5e6ad2', '#fcfcfd', '#1b1b1b', 45), dark: scheme('#606acc', '#0f0f11', '#e3e4e6', 60) },
  { id: 'raycast', label: 'Raycast', light: scheme('#ff6363', '#ffffff', '#030303', 45), dark: scheme('#ff6363', '#101010', '#fefefe', 60) },
  { id: 'vercel', label: 'Vercel', light: scheme('#006aff', '#ffffff', '#171717', 40), dark: scheme('#006efe', '#000000', '#ededed', 50) },
  { id: 'catppuccin', label: 'Catppuccin', light: scheme('#8839ef', '#eff1f5', '#4c4f69', 45), dark: scheme('#cba6f7', '#1e1e2e', '#cdd6f4', 55) },
  { id: 'github', label: 'GitHub', light: scheme('#0969da', '#ffffff', '#1f2328', 42), dark: scheme('#2f81f7', '#0d1117', '#e6edf3', 52) },
  { id: 'gruvbox', label: 'Gruvbox', light: scheme('#af3a03', '#fbf1c7', '#3c3836', 50), dark: scheme('#fe8019', '#282828', '#ebdbb2', 55) },
  { id: 'rose-pine', label: 'Rosé Pine', light: scheme('#907aa9', '#faf4ed', '#575279', 45), dark: scheme('#c4a7e7', '#191724', '#e0def4', 55) },
  { id: 'solarized', label: 'Solarized', light: scheme('#268bd2', '#fdf6e3', '#586e75', 40), dark: scheme('#268bd2', '#002b36', '#93a1a1', 50) },
  { id: 'nord', label: 'Nord', light: scheme('#5e81ac', '#eceff4', '#2e3440', 45), dark: scheme('#88c0d0', '#2e3440', '#eceff4', 50) },
  { id: 'tokyo-night', label: 'Tokyo Night', light: scheme('#34548a', '#d5d6db', '#343b58', 45), dark: scheme('#7aa2f7', '#1a1b26', '#c0caf5', 55) },
  { id: 'everforest', label: 'Everforest', light: scheme('#8da101', '#fdf6e3', '#5c6a72', 45), dark: scheme('#a7c080', '#2d353b', '#d3c6aa', 50) },
  { id: 'one', label: 'One', light: scheme('#4078f2', '#fafafa', '#383a42', 45), dark: scheme('#61afef', '#282c34', '#abb2bf', 50) },
] as const

export const DEFAULT_SCHEME: ModeSchemes = { light: THEME_PRESETS[0].light, dark: THEME_PRESETS[0].dark }

const isHex = (v: unknown): v is string => typeof v === 'string' && /^#[0-9a-fA-F]{6}$/.test(v)

function mergeScheme(base: ColorScheme, c?: ColorSchemeConfig): ColorScheme {
  if (!c) return base
  return {
    accent: isHex(c.accent) ? c.accent.toLowerCase() : base.accent,
    background: isHex(c.background) ? c.background.toLowerCase() : base.background,
    foreground: isHex(c.foreground) ? c.foreground.toLowerCase() : base.foreground,
    contrast:
      typeof c.contrast === 'number' && c.contrast >= 0 && c.contrast <= 100 ? c.contrast : base.contrast,
  }
}

// The deployment default scheme from the build-time config (jaz-defaults.js):
// an optional named preset, then per-mode color overrides, falling back to the
// Jaz default. The user's own choices layer on top of this.
const CONFIG_BASE: ModeSchemes = (() => {
  const cfg = jazDefaults().scheme
  if (!cfg) return DEFAULT_SCHEME
  const base = (cfg.preset && THEME_PRESETS.find((p) => p.id === cfg.preset)) || DEFAULT_SCHEME
  return { light: mergeScheme(base.light, cfg.light), dark: mergeScheme(base.dark, cfg.dark) }
})()

// --- derivation -----------------------------------------------------------

const mix = (toward: string, amount: number, base: string): string =>
  `color-mix(in oklab, ${toward} ${amount.toFixed(1)}%, ${base})`

// sRGB relative luminance, to choose readable text on an accent fill.
function luminance(hex: string): number {
  const h = hex.replace('#', '')
  const v = h.length === 3 ? h.replace(/./g, (c) => c + c) : h
  const ch = (i: number) => {
    const c = parseInt(v.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * ch(0) + 0.7152 * ch(2) + 0.0722 * ch(4)
}

// Pragmatic luminance threshold: above ~0.3 the accent is light enough to need
// dark text (amber, light cyans), while saturated mid-tone blues/purples keep
// the conventional white text. (Pure WCAG crossover 0.179 over-darkens blues.)
const onColor = (hex: string): string => (luminance(hex) > 0.3 ? '#10131a' : '#ffffff')

// Status colors keep a fixed semantic hue across schemes (danger reads red in
// any theme); only their soft tints get mixed onto the scheme background so they
// sit on it correctly. Mid-tone values that read on both light and dark.
const STATUS_DANGER = '#e5484d'
const STATUS_OK = '#30a46c'
const STATUS_RUNNING = '#f2a31e'

// Map a scheme to the full --color-* token set. Surfaces step from the
// background toward the foreground; the ink ramp steps from the foreground
// toward the background; contrast scales the step magnitude.
function tokens(c: ColorScheme): Record<string, string> {
  const { accent, background: bg, foreground: fg } = c
  const k = 0.6 + (c.contrast / 100) * 0.8
  const step = (p: number) => Math.min(60, p * k)
  // The clay accent family mirrors primary in a custom scheme (one accent).
  const strong = mix(fg, 16, accent)
  const soft = mix(accent, 14, bg)
  return {
    bg,
    surface: mix(fg, step(5), bg),
    'surface-2': mix(fg, step(10), bg),
    border: mix(fg, step(16), bg),
    ink: fg,
    'ink-2': mix(bg, step(26), fg),
    'ink-3': mix(bg, step(40), fg),
    primary: accent,
    'primary-strong': strong,
    'primary-soft': soft,
    'on-primary': onColor(accent),
    accent,
    'accent-strong': strong,
    'accent-soft': soft,
    danger: STATUS_DANGER,
    'danger-soft': mix(STATUS_DANGER, 14, bg),
    ok: STATUS_OK,
    running: STATUS_RUNNING,
  }
}

const block = (c: ColorScheme): string =>
  Object.entries(tokens(c))
    .map(([k, v]) => `--color-${k}:${v}`)
    .join(';')

export const sameScheme = (a: ColorScheme, b: ColorScheme): boolean =>
  a.accent === b.accent &&
  a.background === b.background &&
  a.foreground === b.foreground &&
  a.contrast === b.contrast

// Emit a token block only for a mode the user has actually changed, so editing
// light leaves dark on the stock globals.css tokens (and vice versa). Stock in
// both modes yields '' — no override at all.
export function schemeCss(m: ModeSchemes): string {
  const parts: string[] = []
  if (!sameScheme(m.light, DEFAULT_SCHEME.light)) parts.push(`:root{${block(m.light)}}`)
  if (!sameScheme(m.dark, DEFAULT_SCHEME.dark)) parts.push(`:root.dark{${block(m.dark)}}`)
  return parts.join('\n')
}

// --- store -----------------------------------------------------------------

const KEY = 'jaz.appearance.scheme'
// The derived CSS, cached so the pre-paint script in index.html can replay it
// without re-running the derivation in vanilla JS.
const CSS_KEY = 'jaz.appearance.themeCss'
const STYLE_ID = 'jaz-theme-overrides'

const listeners = new Set<() => void>()

// The user's stored override layers over the deployment base (CONFIG_BASE).
const isBase = (m: ModeSchemes): boolean =>
  sameScheme(m.light, CONFIG_BASE.light) && sameScheme(m.dark, CONFIG_BASE.dark)

function readStored(): ModeSchemes {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return CONFIG_BASE
    const p = JSON.parse(raw) as Partial<ModeSchemes>
    return {
      light: { ...CONFIG_BASE.light, ...p.light },
      dark: { ...CONFIG_BASE.dark, ...p.dark },
    }
  } catch {
    return CONFIG_BASE
  }
}

let current: ModeSchemes = readStored()

// Sync the injected <style> to the given CSS (empty ⇒ remove it). Stock keeps
// the hand-tuned globals.css tokens exactly, since no override is emitted.
function render(css: string) {
  const existing = document.getElementById(STYLE_ID)
  if (!css) {
    existing?.remove()
    return
  }
  const el = (existing as HTMLStyleElement | null) ?? document.createElement('style')
  el.id = STYLE_ID
  el.textContent = css
  if (!existing) document.head.appendChild(el)
}

function commit(next: ModeSchemes) {
  current = next
  // Persist the user override only when it diverges from the deployment base, so
  // it keeps tracking the config default otherwise. The derived CSS (vs the Jaz
  // stock tokens) is cached separately for the pre-paint script.
  if (isBase(next)) localStorage.removeItem(KEY)
  else localStorage.setItem(KEY, JSON.stringify(next))
  const css = schemeCss(next)
  if (css) localStorage.setItem(CSS_KEY, css)
  else localStorage.removeItem(CSS_KEY)
  render(css)
  for (const l of listeners) l()
}

export function getScheme(): ModeSchemes {
  return current
}

export function setMode(mode: keyof ModeSchemes, patch: Partial<ColorScheme>) {
  commit({ ...current, [mode]: { ...current[mode], ...patch } })
}

export function applyPreset(mode: keyof ModeSchemes, preset: ThemePreset) {
  commit({ ...current, [mode]: { ...preset[mode] } })
}

export function resetScheme() {
  commit(CONFIG_BASE)
}

window.addEventListener('storage', (event) => {
  if (event.storageArea !== localStorage || event.key !== KEY) return
  current = readStored()
  render(schemeCss(current))
  for (const l of listeners) l()
})

// Reconcile both the cached CSS and the live <style> with the current
// derivation of the stored scheme. Refreshing the cache here keeps the
// pre-paint script accurate across derivation changes (the script already
// injected the previous CSS for first paint).
const initialCss = schemeCss(current)
if (initialCss) localStorage.setItem(CSS_KEY, initialCss)
else localStorage.removeItem(CSS_KEY)
render(initialCss)

function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

export function useScheme() {
  return useSyncExternalStore(subscribe, getScheme)
}
