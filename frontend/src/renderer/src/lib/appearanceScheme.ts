import { useSyncExternalStore } from 'react'
import { jazDefaults } from './jazDefaults'

// The colour theme: up to three linked hues that wash the window, how strongly,
// and a film grain over it. globals.css derives every token from the CSS
// variables this emits, at lightness steps tuned per mode, so text, cards and
// buttons always match whatever colour is picked.
export interface ColorTheme {
  hue: number
  /** 0 is graphite (untinted); 1–3 linked colours */
  colors: number
  /** 0.1–1: how strongly the colours wash the window */
  strength: number
  /** 0–100 */
  grain: number
}

export interface ThemePreset {
  id: string
  label: string
  hue: number
  colors: number
  strength: number
}

// The first entry is the stock look: equal to it, colorCss emits nothing and
// the globals.css defaults rule.
export const THEME_PRESETS: readonly ThemePreset[] = [
  { id: 'cobalt', label: 'Cobalt', hue: 264, colors: 2, strength: 0.6 },
  { id: 'ocean', label: 'Ocean', hue: 205, colors: 2, strength: 0.6 },
  { id: 'forest', label: 'Forest', hue: 150, colors: 2, strength: 0.55 },
  { id: 'sand', label: 'Sand', hue: 82, colors: 2, strength: 0.55 },
  { id: 'ember', label: 'Ember', hue: 48, colors: 2, strength: 0.6 },
  { id: 'rose', label: 'Rose', hue: 355, colors: 2, strength: 0.55 },
  { id: 'aurora', label: 'Aurora', hue: 165, colors: 3, strength: 0.6 },
  { id: 'graphite', label: 'Graphite', hue: 264, colors: 0, strength: 0.6 },
]

export const MAX_COLORS = 3

// The colours are linked: one main hue, the others at fixed turns from it.
export function linkedHues(hue: number, colors: number): number[] {
  return [[], [hue], [hue, hue - 35], [hue, hue + 50, hue - 50]][colors]
}

const fromPreset = ({ hue, colors, strength }: ThemePreset): ColorTheme => ({ hue, colors, strength, grain: 0 })

const STOCK = fromPreset(THEME_PRESETS[0])

export const sameTheme = (a: ColorTheme, b: ColorTheme): boolean =>
  a.hue === b.hue && a.colors === b.colors && a.strength === b.strength && a.grain === b.grain

export const presetMatches = (preset: ThemePreset, theme: ColorTheme): boolean =>
  preset.colors === theme.colors && (!preset.colors || (preset.hue === theme.hue && preset.strength === theme.strength))

const inRange = (value: unknown, min: number, max: number, fallback: number): number =>
  typeof value === 'number' && value >= min && value <= max ? value : fallback

// Loose input (the deploy config, stored JSON) folded onto a base.
function parseTheme(raw: Partial<Record<keyof ColorTheme, unknown>> | undefined, base: ColorTheme): ColorTheme {
  return {
    hue: inRange(raw?.hue, 0, 360, base.hue),
    colors: Math.round(inRange(raw?.colors, 0, MAX_COLORS, base.colors)),
    strength: inRange(raw?.strength, 0.1, 1, base.strength),
    grain: inRange(raw?.grain, 0, 100, base.grain),
  }
}

// The deployment default from jaz-defaults.js: an optional named preset with
// individual values over it. The user's own choice layers on top.
const CONFIG_BASE: ColorTheme = (() => {
  const cfg = jazDefaults().color
  const preset = THEME_PRESETS.find((p) => p.id === cfg?.preset)
  return parseTheme(cfg, preset ? fromPreset(preset) : STOCK)
})()

// The CSS variables globals.css derives every colour from. One colour gets a
// second, slightly turned tone so the window still has depth; graphite washes
// nothing.
export function colorCss(c: ColorTheme): string {
  if (sameTheme(c, STOCK)) return ''
  const [h1 = c.hue, h2 = h1 - 18, h3] = linkedHues(c.hue, c.colors)
  const wash = c.colors ? Math.round(120 + c.strength * 480) / 10 : 0
  const vars = {
    '--hue': c.hue,
    '--tint': c.colors ? 1 : 0,
    '--wash-1': h1,
    '--wash-2': h2,
    '--wash-3': h3 ?? h2,
    '--wash-mix': `${wash}%`,
    '--wash-3-mix': `${h3 === undefined ? 0 : wash}%`,
    '--grain': c.grain / 100,
  }
  return `:root{${Object.entries(vars).map(([k, v]) => `${k}:${v}`).join(';')}}`
}

// --- store -----------------------------------------------------------------

const KEY = 'jaz.appearance.color'
// The CSS, cached so the pre-paint script in index.html can replay it.
const CSS_KEY = 'jaz.appearance.themeCss'
const STYLE_ID = 'jaz-theme-overrides'

const listeners = new Set<() => void>()

function readStored(): ColorTheme {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? parseTheme(JSON.parse(raw), CONFIG_BASE) : CONFIG_BASE
  } catch {
    return CONFIG_BASE
  }
}

let current = readStored()

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

function cache(css: string) {
  if (css) localStorage.setItem(CSS_KEY, css)
  else localStorage.removeItem(CSS_KEY)
}

function commit(next: ColorTheme) {
  current = next
  // A theme left at the deployment base is not stored, so it keeps tracking the base.
  if (sameTheme(next, CONFIG_BASE)) localStorage.removeItem(KEY)
  else localStorage.setItem(KEY, JSON.stringify(next))
  const css = colorCss(next)
  cache(css)
  render(css)
  for (const l of listeners) l()
}

export function getColorTheme(): ColorTheme {
  return current
}

export function setColorTheme(patch: Partial<ColorTheme>) {
  commit({ ...current, ...patch })
}

export function applyPreset(preset: ThemePreset) {
  commit({ ...fromPreset(preset), grain: current.grain })
}

export function resetColorTheme() {
  commit(CONFIG_BASE)
}

window.addEventListener('storage', (event) => {
  if (event.storageArea !== localStorage || event.key !== KEY) return
  current = readStored()
  render(colorCss(current))
  for (const l of listeners) l()
})

// Refresh the cache from the stored theme so the pre-paint script stays
// accurate across builds (it already replayed the previous CSS for first paint).
const initialCss = colorCss(current)
cache(initialCss)
render(initialCss)

function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

export function useColorTheme() {
  return useSyncExternalStore(subscribe, getColorTheme)
}
