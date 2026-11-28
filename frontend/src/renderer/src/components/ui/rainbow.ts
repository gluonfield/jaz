// The app's rainbow vocabulary, shared wherever "alive right now" is drawn.

// A rainbow comet (~100° arc fading in and out of transparency) that orbits a
// card while active; the rest of the perimeter stays a quiet track. Used by
// the composer focus ring, the music bubbles' now-playing ring, and the
// rocket video frame.
export const RAINBOW_BEAM =
  'conic-gradient(from var(--ring-angle, 0deg), transparent 0deg 250deg, var(--color-rainbow-1) 278deg, var(--color-rainbow-2) 296deg, var(--color-rainbow-3) 312deg, var(--color-rainbow-4) 326deg, var(--color-rainbow-5) 340deg, transparent 352deg 360deg)'

// The same comet in the user's own colours, spread evenly along its arc.
export function beam(colors: string[]): string {
  const stops = colors.map((color, i) => `${color} ${278 + (62 * i) / Math.max(1, colors.length - 1)}deg`)
  if (stops.length === 1) stops[0] += ' 340deg'
  return `conic-gradient(from var(--ring-angle, 0deg), transparent 0deg 250deg, ${stops.join(', ')}, transparent 352deg 360deg)`
}

// The fresh-data scanline: a vertical rainbow band swept across a board tile,
// comet-tailed by the mask so only the leading edge glows.
export const SCANLINE_BACKGROUND =
  'linear-gradient(180deg, var(--color-rainbow-1), var(--color-rainbow-2), var(--color-rainbow-3), var(--color-rainbow-4), var(--color-rainbow-5))'
export const SCANLINE_MASK =
  'linear-gradient(90deg, transparent 30%, black 50%, transparent 54%)'
