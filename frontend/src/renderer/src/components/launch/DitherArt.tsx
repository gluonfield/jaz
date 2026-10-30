import { useReducedMotion } from 'motion/react'
import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { dunes, sun } from '@/components/launch/desert'
import { useEffectsEnabled } from '@/lib/appearance'
import { useColorTheme } from '@/lib/appearanceScheme'
import { useTheme } from '@/lib/theme'

// Dithered pixel art: a silhouette is rasterized at grid resolution, then
// ordered-dithered (Bayer + noise) into thousands of tiny square dots in a
// limited brand palette — mostly cobalt, dark grain speckles, rare rainbow
// sparks. The image dissolves in dot by dot, and settled grain keeps quietly
// re-rolling so the artwork feels printed yet alive.
//
// Perf contract: every dot's color is computed once at sample time; the
// dissolve paints only newly-appeared dots each frame (no full-canvas
// repaints); the idle grain repaints only the handful of re-rolled cells per
// tick.
export type Silhouette = (g: OffscreenCanvasRenderingContext2D, w: number, h: number) => void

// prettier-ignore
export const BAYER8 = [
  0, 32, 8, 40, 2, 34, 10, 42,
  48, 16, 56, 24, 50, 18, 58, 26,
  12, 44, 4, 36, 14, 46, 6, 38,
  60, 28, 52, 20, 62, 30, 54, 22,
  3, 35, 11, 43, 1, 33, 9, 41,
  51, 19, 59, 27, 49, 17, 57, 25,
  15, 47, 7, 39, 13, 45, 5, 37,
  63, 31, 55, 23, 61, 29, 53, 21,
]

export function hash(n: number): number {
  let x = (n + 0x9e3779b9) | 0
  x = Math.imul(x ^ (x >>> 16), 0x21f0aaad)
  x = Math.imul(x ^ (x >>> 15), 0x735a2d97)
  return (x ^ (x >>> 15)) >>> 0
}

type Dot = { x: number; y: number; color: string; alpha: number; seed: number; appear: number; ambient: boolean }

type Palette = { primary: string; strong: string; ink: string; rainbow: string[] }

export function readPalette(): Palette {
  const style = getComputedStyle(document.documentElement)
  const read = (name: string) => style.getPropertyValue(name).trim()
  return {
    primary: read('--color-primary'),
    strong: read('--color-primary-strong'),
    ink: read('--color-ink'),
    rainbow: [1, 2, 3, 4, 5].map((i) => read(`--color-rainbow-${i}`)),
  }
}

export function toneFor(seed: number, ambient: boolean, palette: Palette): string {
  const r = seed % 1000
  if (ambient) return r < 300 ? palette.ink : palette.primary
  if (r < 30) return palette.rainbow[seed % 5]
  if (r < 130) return palette.ink
  if (r < 340) return palette.strong
  return palette.primary
}

// The ordered-dither cut: a cell whose coverage reaches it becomes a dot.
export const threshold = (x: number, y: number, seed: number) =>
  0.92 * (0.7 * ((BAYER8[(y % 8) * 8 + (x % 8)] + 0.5) / 64) + 0.3 * ((seed % 4096) / 4096))

function sample(draw: Silhouette, cols: number, rows: number, palette: Palette): Dot[] {
  const off = new OffscreenCanvas(cols, rows)
  const g = off.getContext('2d', { willReadFrequently: true })!
  g.fillStyle = '#fff'
  g.strokeStyle = '#fff'
  draw(g, cols, rows)
  const alpha = g.getImageData(0, 0, cols, rows).data

  const dots: Dot[] = []
  for (let y = 0; y < rows; y++) {
    for (let x = 0; x < cols; x++) {
      const coverage = alpha[(y * cols + x) * 4 + 3] / 255
      const seed = hash(x + y * cols)
      const noise = (seed % 4096) / 4096
      const appear = 0.18 * (x / cols) + (((seed >>> 12) % 1024) / 1024) * 0.95
      const shade = ((seed >>> 4) % 256) / 256
      if (coverage > 0.05) {
        if (coverage < threshold(x, y, seed)) continue
        const base = coverage < 0.6 ? 0.55 + 0.25 * shade : 0.82 + 0.18 * shade
        dots.push({ x, y, color: toneFor(seed, false, palette), alpha: base, seed, appear, ambient: false })
      } else if (noise < 0.012) {
        // Sparse dust around the silhouette keeps the canvas reading as a
        // dithered field rather than a sticker on empty space.
        dots.push({ x, y, color: toneFor(seed, true, palette), alpha: 0.09 + 0.08 * shade, seed, appear, ambient: true })
      }
    }
  }
  dots.sort((a, b) => a.appear - b.appear)
  return dots
}

const GRAIN_EPOCH_MS = 150

// Where a dot of a new mark waits before it flies home, and when (0 to 1)
// in the mark's entrance it sets off. The entrance is the mark's signature:
// the order and the waiting place say what the step is about.
export type Entrance = (x: number, y: number, cols: number, rows: number) => { x: number; y: number; at: number }

// With an entrance, a new silhouette is built from the old one's dots: they
// gather at the new mark's waiting places, then fly home in its entrance
// order and overshoot a little. Spare old dots burst outward and fade; new
// dots without an old one fade in where they wait.
const MORPH_GATHER = 0.3
const MORPH_SPREAD = 0.35
const MORPH_FLIGHT = 0.4
export const MORPH_SECONDS = MORPH_GATHER + MORPH_SPREAD + MORPH_FLIGHT

export const easeInOut = (t: number) => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2)
const overshoot = (t: number) => 1 + 2.4 * (t - 1) ** 3 + 1.4 * (t - 1) ** 2

export const clamp01 = (t: number) => Math.min(1, Math.max(0, t))

type Place = (d: Dot, x: number, y: number, alpha: number) => void

// The morph as a painter of second `t`; it reports whether it is still moving.
// Its last frame is the new art itself.
function morph(from: Dot[], to: Dot[], enter: Entrance, cols: number, rows: number, place: Place) {
  const old = from.filter((d) => !d.ambient).sort((a, b) => a.x - b.x)
  const flights = to
    .filter((d) => !d.ambient)
    .map((d) => ({ d, wait: enter(d.x, d.y, cols, rows) }))
    .sort((a, b) => a.wait.x - b.wait.x)
    .map((f, i, all) => ({ ...f, source: old.length ? old[Math.floor((i * old.length) / all.length)] : undefined }))
  const paired = new Set(flights.map((f) => f.source))
  const spare = old.filter((d) => !paired.has(d))
  const dust = to.filter((d) => d.ambient)
  return (t: number) => {
    const gather = easeInOut(clamp01(t / MORPH_GATHER))
    for (const d of dust) place(d, d.x, d.y, d.alpha)
    for (const d of spare) place(d, d.x + (d.x - cols / 2) * gather, d.y + (d.y - rows / 2) * gather, d.alpha * (1 - gather))
    for (const { d, wait, source } of flights) {
      const gathering = t < MORPH_GATHER
      const [a, b] = gathering ? [source ?? wait, wait] : [wait, d]
      const k = gathering ? gather : overshoot(clamp01((t - MORPH_GATHER - wait.at * MORPH_SPREAD) / MORPH_FLIGHT))
      place(d, a.x + (b.x - a.x) * k, a.y + (b.y - a.y) * k, source ? d.alpha : d.alpha * gather)
    }
    return t < MORPH_SECONDS
  }
}

// The canvas footprint in CSS pixels: `cols × rows` cells, less the trailing gap.
export const ditherSize = (cols: number, rows: number, dot = 3, gap = 1) => ({
  width: cols * (dot + gap) - gap,
  height: rows * (dot + gap) - gap,
})

// Boot art plays its dissolve once per process. The launch screen, the
// onboarding gate, and the onboarding screen each mount their own canvases;
// without this, every handoff replays the build and the boot visibly blinks.
const BUILT_KEYS = new Set<string>()

// Boot art on screen, as the launch splash needs it to land grains on its dots.
export type Landing = { canvas: HTMLCanvasElement; key: string; width: number; pitch: number; dot: number; dots: Dot[] }

// While the launch splash is up it owns the boot reveal: boot art paints built
// at once, underneath, and lists itself so the splash can fly onto it.
export const boot = { splash: false, landings: new Set<Landing>() }

export function DitherArt({
  draw,
  cols,
  rows,
  dot = 3,
  gap = 1,
  delay = 0,
  waitForFonts = false,
  buildKey,
  label,
  enter,
}: {
  draw: Silhouette
  cols: number
  rows: number
  dot?: number
  gap?: number
  delay?: number
  waitForFonts?: boolean
  buildKey?: string
  label?: string
  enter?: Entrance
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const shown = useRef<{ draw: Silhouette; dots: Dot[] }>(undefined)
  const effects = useEffectsEnabled()
  const reduced = useReducedMotion()
  const animate = effects && !reduced
  // The canvas samples the palette at mount; the mode and colour theme re-run it.
  const { resolved } = useTheme()
  const color = useColorTheme()

  const pitch = dot + gap
  const { width, height } = ditherSize(cols, rows, dot, gap)

  // Layout effect so a remount of already-built art paints before the browser
  // does — an effect-timed paint leaves one blank frame, which reads as a blink.
  useLayoutEffect(() => {
    const canvas = canvasRef.current!
    const ctx = canvas.getContext('2d')!
    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.round(width * dpr)
    canvas.height = Math.round(height * dpr)
    ctx.scale(dpr, dpr)

    let cancelled = false
    let raf = 0
    let grainTimer = 0
    let landing: Landing | undefined
    const built = buildKey ? BUILT_KEYS.has(buildKey) : false

    const start = () => {
      if (cancelled) return
      const palette = readPalette()
      const dots = sample(draw, cols, rows, palette)
      const previous = shown.current
      shown.current = { draw, dots }
      if (buildKey) {
        landing = { canvas, key: buildKey, width, pitch, dot, dots }
        boot.landings.add(landing)
      }

      const paintDot = (d: Dot, color: string, alpha: number) => {
        ctx.clearRect(d.x * pitch, d.y * pitch, dot, dot)
        ctx.globalAlpha = alpha
        ctx.fillStyle = color
        ctx.fillRect(d.x * pitch, d.y * pitch, dot, dot)
      }

      const place: Place = (d, x, y, alpha) => {
        ctx.globalAlpha = alpha
        ctx.fillStyle = d.color
        ctx.fillRect(Math.round(x) * pitch, Math.round(y) * pitch, dot, dot)
      }

      const paint = (list: Dot[]) => {
        for (const d of list) place(d, d.x, d.y, d.alpha)
        ctx.globalAlpha = 1
      }

      if (!animate) {
        paint(dots)
        return
      }

      // Idle grain: each tick restores last tick's cells, then re-rolls a
      // small deterministic slice — a crawling-static texture for the cost of
      // a few hundred rect ops per tick.
      let epoch = 0
      let rerolled: Dot[] = []
      const grainTick = () => {
        epoch++
        for (const d of rerolled) paintDot(d, d.color, d.alpha)
        rerolled = []
        const count = Math.max(1, Math.floor(dots.length / 34))
        for (let k = 0; k < count; k++) {
          const d = dots[hash(epoch * 7919 + k) % dots.length]
          const seed = hash(d.seed ^ Math.imul(epoch, 2654435761))
          paintDot(d, toneFor(seed, d.ambient, palette), d.alpha)
          rerolled.push(d)
        }
        ctx.globalAlpha = 1
      }

      const settle = () => {
        if (buildKey) BUILT_KEYS.add(buildKey)
        grainTimer = window.setInterval(grainTick, GRAIN_EPOCH_MS)
      }

      // Runs a painter of seconds since `delay` every frame, from this one on
      // (the resize above already cleared the canvas), until it is done.
      const play = (step: (t: number) => boolean) => {
        const t0 = performance.now()
        const frame = (now: number) => {
          if (step((now - t0) / 1000 - delay)) {
            raf = requestAnimationFrame(frame)
            return
          }
          ctx.globalAlpha = 1
          settle()
        }
        frame(t0)
      }

      if (enter && previous && previous.draw !== draw) {
        const step = morph(previous.dots, dots, enter, cols, rows, place)
        play((t) => {
          ctx.clearRect(0, 0, width, height)
          return step(t)
        })
      } else if (built || (buildKey && boot.splash)) {
        paint(dots)
        settle()
      } else {
        let index = 0
        play((t) => {
          let next = index
          while (next < dots.length && dots[next].appear <= t) next++
          paint(dots.slice(index, next))
          index = next
          return index < dots.length
        })
      }
    }

    // Once built, fonts are certainly loaded — start synchronously so the
    // repaint happens inside this layout effect, not a microtask later.
    if (waitForFonts && !built) {
      void document.fonts.ready.then(() => start())
    } else {
      start()
    }
    return () => {
      cancelled = true
      cancelAnimationFrame(raf)
      window.clearInterval(grainTimer)
      if (landing) boot.landings.delete(landing)
    }
  }, [animate, draw, enter, cols, rows, dot, gap, pitch, width, height, delay, waitForFonts, buildKey, resolved, color])

  return (
    <canvas
      ref={canvasRef}
      role="img"
      aria-label={label}
      aria-hidden={label ? undefined : true}
      style={{ width, maxWidth: '100%', marginInline: 'auto', display: 'block' }}
    />
  )
}

export const WORDMARK_FONT = "600 100px 'Inter Variable', 'Inter', sans-serif"
const WORDMARK_ROWS = 48

export const drawText =
  (text: string): Silhouette =>
  (g, w, h) => {
    g.font = WORDMARK_FONT
    g.textAlign = 'center'
    const probe = g.measureText(text)
    const probeHeight = probe.actualBoundingBoxAscent + probe.actualBoundingBoxDescent
    const size = 100 * Math.min((0.9 * w) / probe.width, (0.88 * h) / probeHeight)
    g.font = WORDMARK_FONT.replace('100px', `${size}px`)
    const m = g.measureText(text)
    g.fillText(text, w / 2, h / 2 + (m.actualBoundingBoxAscent - m.actualBoundingBoxDescent) / 2)
  }

// Columns that hold the glyph height steady as the text grows, so a phrase
// widens instead of shrinking into grain; "jaz" sets the floor.
function wordmarkCols(text: string): number {
  const g = new OffscreenCanvas(1, 1).getContext('2d')!
  g.font = WORDMARK_FONT
  const m = g.measureText(text)
  const aspect = m.width / (m.actualBoundingBoxAscent + m.actualBoundingBoxDescent)
  return Math.max(112, Math.ceil((0.88 * WORDMARK_ROWS * aspect) / 0.9))
}

// A wordmark dissolving in as dithered brand grain: "jaz" on the boot and
// onboarding screens, the user's own text above the new-thread composer. Under
// a `maxWidth`, longer text steps to a finer grain rather than growing past it;
// past that, the canvas scales down as one image (its max-width).
export function DitherWordmark({ text = 'jaz', delay = 0, maxWidth = Infinity }: { text?: string; delay?: number; maxWidth?: number }) {
  const { draw, cols, dot } = useMemo(() => {
    const cols = wordmarkCols(text)
    const dot = [3, 2].find((size) => cols * (size + 1) - 1 <= maxWidth) ?? 2
    return { draw: drawText(text), cols, dot }
  }, [text, maxWidth])
  return (
    <DitherArt draw={draw} cols={cols} rows={WORDMARK_ROWS} dot={dot} delay={delay} waitForFonts buildKey={`wordmark:${text}`} label={text} />
  )
}

const TERRAIN_ROWS = 44
const SKY_ROWS = 26
type Sky = 'sun' | 'moon'

const cell = (g: OffscreenCanvasRenderingContext2D, x: number, y: number, coverage: number) => {
  g.globalAlpha = Math.min(1, Math.max(0, coverage))
  g.fillRect(x, y, 1, 1)
}

const drawTerrain: Silhouette = (g, w, h) => {
  dunes(w, h, (x, y, coverage) => cell(g, x, y, coverage))
  g.globalAlpha = 1
}

// The dunes under a theme-keyed sky, in cell units: a banded sun by day, a moon
// and stars by night, right of centre and behind the far crests.
const drawScene =
  (sky: Sky): Silhouette =>
  (g, w, h) => {
    const ground = h - TERRAIN_ROWS
    const horizon = dunes(w, TERRAIN_ROWS, (x, y, coverage) => cell(g, x, ground + y, coverage))
    const open = (x: number, y: number) => x >= 0 && x < w && y >= 0 && y < ground + horizon[x]
    const cx = Math.round(w * 0.82)
    sun(cx, 18, 10, sky === 'moon', (x, y, coverage) => open(x, y) && cell(g, x, y, coverage))
    if (sky === 'moon') {
      for (let i = 0; i < 24; i++) {
        const x = hash(i * 131 + 3) % w
        const y = hash(i * 197 + 5) % SKY_ROWS
        if (Math.hypot(x - cx, y - 18) < 16 || !open(x, y)) continue
        cell(g, x, y, 1)
      }
    }
    g.globalAlpha = 1
  }

const SCENES: Record<Sky, Silhouette> = { sun: drawScene('sun'), moon: drawScene('moon') }

// Full-bleed dithered brandscape for the bottom of launch/onboarding screens
// and, under a `sky` that follows the theme, the new-thread home. Sized to its
// container; a resize re-dissolves the terrain at the new width.
export function DitherTerrain({ className = '', sky = false, delay = 0 }: { className?: string; sky?: boolean; delay?: number }) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const { resolved } = useTheme()
  const [cols, setCols] = useState(0)
  useLayoutEffect(() => {
    const el = wrapRef.current!
    // Size from the content box (padding excluded) so a padded wrapper can
    // never feed its own overflow back into a grow loop. The observer fires
    // once on observe, so no manual initial measure.
    const observer = new ResizeObserver((entries) => {
      const width = entries[entries.length - 1].contentRect.width
      setCols(Math.max(1, Math.floor(width / 4)))
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])
  return (
    <div ref={wrapRef} aria-hidden className={`pointer-events-none overflow-hidden ${className}`}>
      {cols > 1 ? (
        <DitherArt
          draw={sky ? SCENES[resolved === 'dark' ? 'moon' : 'sun'] : drawTerrain}
          cols={cols}
          rows={sky ? TERRAIN_ROWS + SKY_ROWS : TERRAIN_ROWS}
          delay={delay}
          buildKey={sky ? `jaz-terrain-${resolved}` : 'jaz-terrain'}
        />
      ) : null}
    </div>
  )
}
