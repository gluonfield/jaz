import { useSyncExternalStore } from 'react'

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
// stock globals.css tokens rule.
export const THEME_PRESETS: readonly ThemePreset[] = [
  { id: 'jaz', label: 'Jaz', light: scheme('#3b5bdb', '#eef0f5', '#2a2e3a', 45), dark: scheme('#8aa6ff', '#1b1d24', '#edf0f5', 55) },
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

// Map a scheme to the full --color-* token set. Surfaces step from the
// background toward the foreground; the ink ramp steps from the foreground
// toward the background; contrast scales the step magnitude.
function tokens(c: ColorScheme): Record<string, string> {
  const { accent, background: bg, foreground: fg } = c
  const k = 0.6 + (c.contrast / 100) * 0.8
  const step = (p: number) => Math.min(60, p * k)
  return {
    bg,
    surface: mix(fg, step(5), bg),
    'surface-2': mix(fg, step(10), bg),
    border: mix(fg, step(16), bg),
    ink: fg,
    'ink-2': mix(bg, step(26), fg),
    'ink-3': mix(bg, step(40), fg),
    primary: accent,
    'primary-strong': mix(fg, 16, accent),
    'primary-soft': mix(accent, 14, bg),
    'on-primary': onColor(accent),
    accent,
    'accent-strong': mix(fg, 16, accent),
    'accent-soft': mix(accent, 14, bg),
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

function readStored(): ModeSchemes {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return DEFAULT_SCHEME
    const p = JSON.parse(raw) as Partial<ModeSchemes>
    return {
      light: { ...DEFAULT_SCHEME.light, ...p.light },
      dark: { ...DEFAULT_SCHEME.dark, ...p.dark },
    }
  } catch {
    return DEFAULT_SCHEME
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
  const css = schemeCss(next)
  // Cache both the scheme (for the editor) and the derived CSS (for the
  // pre-paint script). Empty CSS means stock — drop both keys.
  if (css) {
    localStorage.setItem(KEY, JSON.stringify(next))
    localStorage.setItem(CSS_KEY, css)
  } else {
    localStorage.removeItem(KEY)
    localStorage.removeItem(CSS_KEY)
  }
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
  commit(DEFAULT_SCHEME)
}

window.addEventListener('storage', (event) => {
  if (event.storageArea !== localStorage || event.key !== KEY) return
  current = readStored()
  render(schemeCss(current))
  for (const l of listeners) l()
})

// Reconcile the cached <style> with the stored scheme (the pre-paint script
// already injected it for first paint).
render(schemeCss(current))

function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

export function useScheme() {
  return useSyncExternalStore(subscribe, getScheme)
}
