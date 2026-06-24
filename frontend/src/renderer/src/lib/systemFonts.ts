import { useCallback, useState } from 'react'

// Installed-font enumeration via the Local Font Access API (queryLocalFonts).
// The main process grants the 'local-fonts' permission; the API also needs
// transient user activation, so callers trigger load() from a real gesture
// (focusing the field) rather than on mount.

export interface SystemFonts {
  /** every installed family, de-duped and sorted */
  all: string[]
  /** the subset whose glyphs share one advance width (monospace) */
  mono: string[]
}

const EMPTY: SystemFonts = { all: [], mono: [] }

type FontData = { family: string }
type QueryLocalFonts = () => Promise<FontData[]>

let cache: SystemFonts | null = null
let inflight: Promise<SystemFonts> | null = null

// A font is monospace when 'i' and 'm' (extreme-width glyphs in proportional
// faces) resolve to the same advance. Measured against the actually-installed
// family, so an unavailable name falls back to the canvas default and is
// (correctly) not flagged monospace.
function detectMonospace(families: string[]): string[] {
  const ctx = document.createElement('canvas').getContext('2d')
  if (!ctx) return []
  const mono: string[] = []
  for (const family of families) {
    const quoted = `"${family.replace(/"/g, '')}"`
    ctx.font = `16px ${quoted}`
    const narrow = ctx.measureText('iiiiiiiiii').width
    const wide = ctx.measureText('mmmmmmmmmm').width
    if (narrow > 0 && Math.abs(narrow - wide) < 0.5) mono.push(family)
  }
  return mono
}

async function fetchFonts(): Promise<SystemFonts> {
  const query = (window as unknown as { queryLocalFonts?: QueryLocalFonts }).queryLocalFonts
  if (typeof query !== 'function') return EMPTY
  try {
    const data = await query()
    const all = Array.from(new Set(data.map((d) => d.family).filter(Boolean))).sort((a, b) =>
      a.localeCompare(b),
    )
    return { all, mono: detectMonospace(all) }
  } catch {
    // Permission denied, no user activation, or unsupported — callers degrade
    // to their built-in suggestions.
    return EMPTY
  }
}

export function loadSystemFonts(): Promise<SystemFonts> {
  if (cache) return Promise.resolve(cache)
  inflight ??= fetchFonts().then((result) => {
    cache = result
    return result
  })
  return inflight
}

// Returns the cached fonts plus a load() to kick off enumeration from a user
// gesture. Safe to call load() repeatedly; the fetch runs once per session.
export function useSystemFonts() {
  const [fonts, setFonts] = useState<SystemFonts>(cache ?? EMPTY)
  const load = useCallback(() => {
    if (cache) {
      setFonts(cache)
      return
    }
    void loadSystemFonts().then(setFonts)
  }, [])
  return { fonts, load }
}
