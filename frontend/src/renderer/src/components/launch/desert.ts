// The brand desert, shared by the launch splash and the boot screens' terrain:
// three dune ranges with sharp crests, lit and shaded faces near each crest
// thinning into haze below, and a sun (a moon by night) with a faint halo.
// Dune periods scale with the band's height, so a thin strip and a whole
// window show the same desert.

const RANGES = [
  { base: 0.21, amp: 0.21, period: 1.3, phase: 2.2, density: 0.5 },
  { base: 0.5, amp: 0.25, period: 2, phase: 0.4, density: 0.62 },
  { base: 0.83, amp: 0.31, period: 3, phase: 1.5, density: 0.85 },
]

const dune = (u: number) =>
  0.75 * (1 - Math.abs(Math.sin(u + 0.35 * Math.sin(u)))) + 0.25 * (1 - Math.abs(Math.sin(2.7 * u + 1)))

// Visits each cell the ranges cover in a band `height` rows tall, with its
// coverage and range (0 is the farthest). Returns the far crest row per
// column: the sky's horizon.
export function dunes(
  cols: number,
  height: number,
  visit: (x: number, y: number, coverage: number, range: number) => void,
): Float32Array {
  const tops = RANGES.map((range) =>
    Float32Array.from({ length: cols + 2 }, (_, c) => {
      const u = ((c - 1) / (height * range.period)) * Math.PI + range.phase
      return height * (range.base - range.amp * dune(u))
    }),
  )
  const covered = new Float32Array(cols).fill(height)
  for (let i = RANGES.length - 1; i >= 0; i--) {
    const top = tops[i]
    for (let x = 0; x < cols; x++) {
      const crest = Math.ceil(top[x + 1])
      const facing = Math.max(-1, Math.min(1, 1.5 * (top[x + 2] - top[x])))
      for (let y = crest; y < covered[x]; y++) {
        const depth = y - crest
        const haze = RANGES[i].density * (0.55 + 0.45 * Math.exp(-depth / 14))
        visit(x, y, depth === 0 ? 1 : haze + 0.35 * facing * Math.exp(-depth / 6), i)
      }
      covered[x] = Math.min(covered[x], crest)
    }
  }
  return Float32Array.from({ length: cols }, (_, x) => Math.ceil(tops[0][x + 1]))
}

// Visits the cells of a sun centred on (cx, cy): by day a disc cut by bands
// that thicken toward the horizon, by night a moon lit from the upper left;
// a halo either way. `d` is the cell's distance from the centre in radii.
export function sun(
  cx: number,
  cy: number,
  radius: number,
  dark: boolean,
  visit: (x: number, y: number, coverage: number, d: number) => void,
) {
  for (let y = Math.floor(cy - 1.5 * radius); y < cy + 1.5 * radius; y++) {
    for (let x = Math.floor(cx - 1.5 * radius); x < cx + 1.5 * radius; x++) {
      const dx = (x - cx) / radius
      const dy = (y - cy) / radius
      const d = Math.hypot(dx, dy)
      if (d > 1.5) continue
      const lit = Math.max(0, Math.min(1, -0.55 * dx - 0.35 * dy + 0.75 * Math.sqrt(Math.max(0, 1 - d * d))))
      const banded = dy > 0.05 && (dy * 5 + 0.3) % 1 < dy * 0.6
      const coverage =
        d > 1 ? (dark ? 0.16 : 0.3) * (1 - (d - 1) / 0.5) ** 2 : dark ? 0.15 + 0.85 * lit : banded ? 0 : 1
      visit(x, y, coverage, d)
    }
  }
}
