import { useSyncExternalStore } from 'react'
import { jazDefaults } from './jazDefaults'

// The colour theme, as on Arc and Zen's picker: a main hue with up to two more
// linked to it, how saturated they are, how strongly they wash the window, and
// a film grain over it. globals.css derives every token from the CSS variables
// this emits, at lightness steps tuned per mode, so text, cards and buttons
// always match whatever colour is picked.
export interface ColorTheme {
  hue: number
  /** 0 is graphite (untinted); 1–3 linked colours */
  colors: number
  /** Which way the colours are linked: an index into HARMONIES[colors] */
  harmony: number
  /** 0–1: how saturated the colours are */
  saturation: number
  /** 0–1: how strongly they wash the window */
  strength: number
  /** 0–1: the grain's opacity */
  grain: number
}

export const MAX_COLORS = 3

// Zen's ways of linking colours, as turns from the main hue, per colour count.
export const HARMONIES: readonly (readonly { label: string; turns: readonly number[] }[])[] = [
  [],
  [{ label: 'Single', turns: [] }],
  [
    { label: 'Complementary', turns: [180] },
    { label: 'Analogous', turns: [-50] },
  ],
  [
    { label: 'Analogous', turns: [50, -50] },
    { label: 'Split complementary', turns: [150, 210] },
    { label: 'Triadic', turns: [120, 240] },
  ],
]

export const harmonyOf = ({ colors, harmony }: Pick<ColorTheme, 'colors' | 'harmony'>) =>
  HARMONIES[colors][harmony % HARMONIES[colors].length]

export function linkedHues(c: Pick<ColorTheme, 'hue' | 'colors' | 'harmony'>): number[] {
  return c.colors ? [c.hue, ...harmonyOf(c).turns.map((turn) => c.hue + turn)] : []
}

// Chroma for a saturation, shared by the window washes and the picker's swatches.
export const washChroma = (saturation: number) => 0.03 + saturation * 0.15

// The stock look: equal to it, colorCss emits nothing and the globals.css defaults rule.
const STOCK: ColorTheme = { hue: 264, colors: 2, harmony: 1, saturation: 0.6, strength: 0.45, grain: 0 }

export const sameTheme = (a: ColorTheme, b: ColorTheme): boolean =>
  (Object.keys(STOCK) as (keyof ColorTheme)[]).every((key) => a[key] === b[key])

const inRange = (value: unknown, min: number, max: number, fallback: number): number =>
  typeof value === 'number' && value >= min && value <= max ? value : fallback

// Loose input (the deploy config, stored JSON) folded onto a base.
function parseTheme(raw: Partial<Record<keyof ColorTheme, unknown>> | undefined, base: ColorTheme): ColorTheme {
  return {
    hue: inRange(raw?.hue, 0, 360, base.hue),
    colors: Math.round(inRange(raw?.colors, 0, MAX_COLORS, base.colors)),
    harmony: Math.round(inRange(raw?.harmony, 0, 2, base.harmony)),
    saturation: inRange(raw?.saturation, 0, 1, base.saturation),
    strength: inRange(raw?.strength, 0, 1, base.strength),
    grain: inRange(raw?.grain, 0, 1, base.grain),
  }
}

// The deployment default from jaz-defaults.js; the user's own choice layers on top.
const CONFIG_BASE = parseTheme(jazDefaults().color, STOCK)

// The CSS variables globals.css derives every colour from. With fewer than
// three colours the last one fills the remaining wash layers; graphite washes
// nothing.
export function colorCss(c: ColorTheme): string {
  if (sameTheme(c, STOCK)) return ''
  const [h1 = c.hue, h2 = h1, h3 = h2] = linkedHues(c)
  const vars = {
    '--hue': c.hue,
    '--tint': c.colors ? 1 : 0,
    '--wash-1': h1,
    '--wash-2': h2,
    '--wash-3': h3,
    '--wash-chroma': washChroma(c.saturation).toFixed(3),
    '--wash-mix': `${c.colors ? Math.round(15 + c.strength * 60) : 0}%`,
    '--grain': c.grain,
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
