// The app's rainbow vocabulary, shared wherever "alive right now" is drawn.

// A rainbow comet (~100° arc fading in and out of transparency) that orbits a
// card while active; the rest of the perimeter stays a quiet track. Used by
// the composer focus ring, the music bubbles' now-playing ring, and the
// rocket video frame.
export const RAINBOW_BEAM =
  'conic-gradient(from var(--ring-angle, 0deg), transparent 0deg 250deg, var(--color-rainbow-1) 278deg, var(--color-rainbow-2) 296deg, var(--color-rainbow-3) 312deg, var(--color-rainbow-4) 326deg, var(--color-rainbow-5) 340deg, transparent 352deg 360deg)'

// The fresh-data scanline: a vertical rainbow band swept across a widget,
// comet-tailed by the mask so only the leading edge glows.
export const SCANLINE_BACKGROUND =
  'linear-gradient(180deg, var(--color-rainbow-1), var(--color-rainbow-2), var(--color-rainbow-3), var(--color-rainbow-4), var(--color-rainbow-5))'
export const SCANLINE_MASK =
  'linear-gradient(90deg, transparent 30%, black 50%, transparent 54%)'
