// The brand scenery, shared by the launch splash and the boot screens' terrain:
// ranges with sharp crests, lit and shaded faces near each crest thinning into
// haze below, under a sun (a moon by night) with a faint halo. Each range's
// features scale with the band's height, so a thin strip and a whole window
// show the same scene.

// A range's top row per column, one spare column on each side, and the
// coverage of a cell `depth` rows below it, where `facing` is the slope there.
type Range = {
  tops: (cols: number, height: number) => Float32Array
  face: (x: number, depth: number, facing: number) => number
}

const dune = (u: number) =>
  0.75 * (1 - Math.abs(Math.sin(u + 0.35 * Math.sin(u)))) + 0.25 * (1 - Math.abs(Math.sin(2.7 * u + 1)))

const swell = (base: number, amp: number, period: number, phase: number, density: number): Range => ({
  face: (_, depth, facing) => density * (0.55 + 0.45 * Math.exp(-depth / 14)) + 0.35 * facing * Math.exp(-depth / 6),
  tops: (cols, height) =>
    Float32Array.from({ length: cols + 2 }, (_, c) => {
      const u = ((c - 1) / (height * period)) * Math.PI + phase
      return height * (base - amp * dune(u))
    }),
})

const random = (n: number) => {
  const s = Math.sin(n * 127.1) * 43758.5453
  return s - Math.floor(s)
}

// A street of towers, a few of them tall and some with a spire, each with a
// shaded side and windows lit at random.
const skyline = (base: number, amp: number, seed: number, density: number): Range => ({
  face: (x, depth, facing) =>
    facing > 0 ? 1 : x % 3 && depth % 4 > 1 ? Math.round(random(seed + Math.floor(x / 3) + 57 * Math.floor(depth / 4))) : density,
  tops: (cols, height) => {
    const tops = new Float32Array(cols + 2)
    for (let c = 0, block = seed; c < tops.length; block++) {
      const width = Math.ceil(height * (0.1 + 0.16 * random(block)))
      const top = height * (base - amp * random(block + 0.5) ** 2)
      tops.fill(top, c, c + width)
      if (random(block + 0.25) < 0.35) tops[c + (width >> 1)] = top - 0.14 * height
      c += width
    }
    return tops
  },
})

// The limb of a banded planet seen from orbit, lit from the left.
const limb: Range = {
  face: (_, depth, facing) => (0.5 - 0.4 * facing) * (0.7 + 0.3 * Math.sin(0.12 * depth * depth ** 0.5)),
  tops: (cols, height) =>
    Float32Array.from({ length: cols + 2 }, (_, c) => 0.12 * height + 1.2 * cols - 1.2 * cols * Math.sqrt(1 - ((c - 1 - cols / 2) / (1.2 * cols)) ** 2)),
}

// Visits each cell the ranges cover in a band `height` rows tall, with its
// coverage and range (0 is the farthest). Returns the highest covered row per
// column: the sky's horizon.
export type Ground = (
  cols: number,
  height: number,
  visit: (x: number, y: number, coverage: number, range: number) => void,
) => Float32Array

const ground =
  (ranges: Range[]): Ground =>
  (cols, height, visit) => {
    const covered = new Float32Array(cols).fill(height)
    for (let i = ranges.length - 1; i >= 0; i--) {
      const top = ranges[i].tops(cols, height)
      for (let x = 0; x < cols; x++) {
        const crest = Math.ceil(top[x + 1])
        const facing = Math.max(-1, Math.min(1, 1.5 * (top[x + 2] - top[x])))
        for (let y = crest; y < covered[x]; y++) {
          visit(x, y, y === crest ? 1 : ranges[i].face(x, y - crest, facing), i)
        }
        covered[x] = Math.min(covered[x], crest)
      }
    }
    return covered
  }

// A scene's ground and its sun: how far across it sits, its size, and whether
// the sky is night whatever the theme.
export type Scene = { ground: Ground; sky: { x: number; size: number; night?: true } }

export const SCENES: Record<'blank' | 'desert' | 'city' | 'orbit', Scene | null> = {
  blank: null,
  desert: {
    ground: ground([swell(0.21, 0.21, 1.3, 2.2, 0.5), swell(0.5, 0.25, 2, 0.4, 0.62), swell(0.83, 0.31, 3, 1.5, 0.85)]),
    sky: { x: 0.8, size: 1 },
  },
  city: { ground: ground([skyline(0.5, 0.5, 11, 0.4), skyline(0.85, 0.6, 47, 1)]), sky: { x: 0.4, size: 1.7 } },
  orbit: { ground: ground([limb]), sky: { x: 0.2, size: 0.45, night: true } },
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
