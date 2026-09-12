import { useReducedMotion } from 'motion/react'
import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useEffectsEnabled } from '@/lib/appearance'
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
const BAYER8 = [
  0, 32, 8, 40, 2, 34, 10, 42,
  48, 16, 56, 24, 50, 18, 58, 26,
  12, 44, 4, 36, 14, 46, 6, 38,
  60, 28, 52, 20, 62, 30, 54, 22,
  3, 35, 11, 43, 1, 33, 9, 41,
  51, 19, 59, 27, 49, 17, 57, 25,
  15, 47, 7, 39, 13, 45, 5, 37,
  63, 31, 55, 23, 61, 29, 53, 21,
]

function hash(n: number): number {
  let x = (n + 0x9e3779b9) | 0
  x = Math.imul(x ^ (x >>> 16), 0x21f0aaad)
  x = Math.imul(x ^ (x >>> 15), 0x735a2d97)
  return (x ^ (x >>> 15)) >>> 0
}

type Dot = { x: number; y: number; color: string; alpha: number; seed: number; appear: number; ambient: boolean }

type Palette = { primary: string; strong: string; ink: string; rainbow: string[] }

function readPalette(): Palette {
  const style = getComputedStyle(document.documentElement)
  const read = (name: string) => style.getPropertyValue(name).trim()
  return {
    primary: read('--color-primary'),
    strong: read('--color-primary-strong'),
    ink: read('--color-ink'),
    rainbow: [1, 2, 3, 4, 5].map((i) => read(`--color-rainbow-${i}`)),
  }
}

function toneFor(seed: number, ambient: boolean, palette: Palette): string {
  const r = seed % 1000
  if (ambient) return r < 300 ? palette.ink : palette.primary
  if (r < 30) return palette.rainbow[seed % 5]
  if (r < 130) return palette.ink
  if (r < 340) return palette.strong
  return palette.primary
}

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
        const threshold = 0.92 * (0.7 * ((BAYER8[(y % 8) * 8 + (x % 8)] + 0.5) / 64) + 0.3 * noise)
        if (coverage < threshold) continue
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

// Boot art plays its dissolve once per process. The launch screen, the
// onboarding gate, and the onboarding screen each mount their own canvases;
// without this, every handoff replays the build and the boot visibly blinks.
const BUILT_KEYS = new Set<string>()

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
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const effects = useEffectsEnabled()
  const reduced = useReducedMotion()
  const animate = effects && !reduced
  // The canvas samples the palette at mount; the theme key re-runs on flip.
  const { resolved } = useTheme()

  const pitch = dot + gap
  const width = cols * pitch - gap
  const height = rows * pitch - gap

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
    const alreadyBuilt = buildKey ? BUILT_KEYS.has(buildKey) : false

    const start = () => {
      if (cancelled) return
      const palette = readPalette()
      const dots = sample(draw, cols, rows, palette)

      const paintDot = (d: Dot, color: string, alpha: number) => {
        ctx.clearRect(d.x * pitch, d.y * pitch, dot, dot)
        ctx.globalAlpha = alpha
        ctx.fillStyle = color
        ctx.fillRect(d.x * pitch, d.y * pitch, dot, dot)
      }

      const paintRange = (from: number, to: number) => {
        for (let i = from; i < to; i++) {
          const d = dots[i]
          ctx.globalAlpha = d.alpha
          ctx.fillStyle = d.color
          ctx.fillRect(d.x * pitch, d.y * pitch, dot, dot)
        }
        ctx.globalAlpha = 1
      }

      if (!animate) {
        paintRange(0, dots.length)
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

      if (alreadyBuilt) {
        paintRange(0, dots.length)
        grainTimer = window.setInterval(grainTick, GRAIN_EPOCH_MS)
        return
      }

      let index = 0
      const t0 = performance.now()
      const frame = (now: number) => {
        const t = (now - t0) / 1000 - delay
        let next = index
        while (next < dots.length && dots[next].appear <= t) next++
        if (next > index) {
          paintRange(index, next)
          index = next
        }
        if (index < dots.length) {
          raf = requestAnimationFrame(frame)
        } else {
          if (buildKey) BUILT_KEYS.add(buildKey)
          grainTimer = window.setInterval(grainTick, GRAIN_EPOCH_MS)
        }
      }
      raf = requestAnimationFrame(frame)
    }

    // Once built, fonts are certainly loaded — start synchronously so the
    // repaint happens inside this layout effect, not a microtask later.
    if (waitForFonts && !alreadyBuilt) {
      void document.fonts.ready.then(() => start())
    } else {
      start()
    }
    return () => {
      cancelled = true
      cancelAnimationFrame(raf)
      window.clearInterval(grainTimer)
    }
  }, [animate, draw, cols, rows, dot, gap, pitch, width, height, delay, waitForFonts, buildKey, resolved])

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

const WORDMARK_FONT = "600 100px 'Inter Variable', 'Inter', sans-serif"
const WORDMARK_ROWS = 48

const drawText =
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

const TERRAIN_SEED = 77

// Procedural ridgelines: layered absolute-sine ridges, back range sparse and
// tall, front range dense. Deterministic, so the brandscape is the same on
// every boot.
const drawTerrain: Silhouette = (g, w, h) => {
  const ridge = (u: number, s1: number, s2: number, s3: number) =>
    0.55 * (1 - Math.abs(Math.sin(u * 2.1 + s1))) +
    0.3 * (1 - Math.abs(Math.sin(u * 5.3 + s2))) +
    0.15 * (1 - Math.abs(Math.sin(u * 12.7 + s3)))
  const layer = (seed: number, lift: number, alpha: number) => {
    const s1 = (hash(seed) % 628) / 100
    const s2 = (hash(seed + 1) % 628) / 100
    const s3 = (hash(seed + 2) % 628) / 100
    g.globalAlpha = alpha
    g.beginPath()
    g.moveTo(0, h)
    for (let x = 0; x <= w; x++) {
      const r = ridge(x / (h * 4), s1, s2, s3)
      g.lineTo(x, h * (1 - lift * (0.2 + 0.8 * r)))
    }
    g.lineTo(w, h)
    g.closePath()
    g.fill()
  }
  layer(TERRAIN_SEED, 0.95, 0.4)
  layer(TERRAIN_SEED + 9, 0.55, 1)
  g.globalAlpha = 1
}

const TERRAIN_ROWS = 44
const SKY_ROWS = 26
type Sky = 'sun' | 'moon'

// Theme-keyed sky over the ridgelines, in cell units: a dithered sun by day, a
// crescent with stars by night. Anchored right of center, above the front range.
const drawSky =
  (sky: Sky): Silhouette =>
  (g, w, h) => {
    const cx = Math.round(w * 0.82)
    const cy = 18
    const r = 10
    const disc = (x: number, y: number, radius: number) => {
      g.beginPath()
      g.arc(x, y, radius, 0, Math.PI * 2)
      g.fill()
    }
    if (sky === 'sun') {
      g.globalAlpha = 0.85
      disc(cx, cy, r)
      g.lineWidth = 2
      for (let i = 0; i < 8; i++) {
        const a = (i / 8) * Math.PI * 2
        g.beginPath()
        g.moveTo(cx + Math.cos(a) * (r + 3), cy + Math.sin(a) * (r + 3))
        g.lineTo(cx + Math.cos(a) * (r + 7), cy + Math.sin(a) * (r + 7))
        g.stroke()
      }
      g.globalAlpha = 1
      return
    }
    disc(cx, cy, r)
    g.globalCompositeOperation = 'destination-out'
    disc(cx + 5, cy - 3, r - 1)
    g.globalCompositeOperation = 'source-over'
    for (let i = 0; i < 24; i++) {
      const x = hash(i * 131 + 3) % w
      const y = hash(i * 197 + 5) % h
      if (Math.hypot(x - cx, y - cy) < r + 4) continue
      const size = i % 6 === 0 ? 2 : 1
      g.fillRect(x, y, size, size)
    }
  }

const drawScene =
  (sky: Sky): Silhouette =>
  (g, w, h) => {
    drawSky(sky)(g, w, SKY_ROWS)
    g.translate(0, h - TERRAIN_ROWS)
    drawTerrain(g, w, TERRAIN_ROWS)
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
