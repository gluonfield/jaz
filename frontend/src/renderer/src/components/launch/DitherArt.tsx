import { useReducedMotion } from 'motion/react'
import { useEffect, useRef } from 'react'
import { useEffectsEnabled } from '@/lib/appearance'
import { useTheme } from '@/lib/theme'

// Dithered pixel art: a silhouette is rasterized at grid resolution, then
// ordered-dithered (Bayer + noise) into thousands of tiny square dots in a
// limited brand palette — mostly cobalt, dark grain speckles, rare rainbow
// sparks. The image dissolves in dot by dot, and settled grain keeps quietly
// re-rolling so the artwork feels printed yet alive.
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

type Dot = {
  x: number
  y: number
  edge: boolean
  ambient: boolean
  seed: number
  appear: number
}

function sample(draw: Silhouette, cols: number, rows: number): Dot[] {
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
      if (coverage > 0.05) {
        const threshold = 0.92 * (0.7 * ((BAYER8[(y % 8) * 8 + (x % 8)] + 0.5) / 64) + 0.3 * noise)
        if (coverage < threshold) continue
        dots.push({ x, y, edge: coverage < 0.6, ambient: false, seed, appear })
      } else if (noise < 0.012) {
        // Sparse dust around the silhouette keeps the canvas reading as a
        // dithered field rather than a sticker on empty space.
        dots.push({ x, y, edge: false, ambient: true, seed, appear })
      }
    }
  }
  return dots
}

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

function tone(dot: Dot, seed: number, palette: Palette): string {
  const r = seed % 1000
  if (dot.ambient) return r < 300 ? palette.ink : palette.primary
  if (r < 30) return palette.rainbow[seed % 5]
  if (r < 130) return palette.ink
  if (r < 340) return palette.strong
  return palette.primary
}

function baseAlpha(dot: Dot, seed: number): number {
  const noise = ((seed >>> 4) % 256) / 256
  if (dot.ambient) return 0.09 + 0.08 * noise
  if (dot.edge) return 0.55 + 0.25 * noise
  return 0.82 + 0.18 * noise
}

const BUILD_END = 1.4
const GRAIN_EPOCH = 0.15

export function DitherArt({
  draw,
  cols,
  rows,
  dot = 3,
  gap = 1,
  delay = 0,
  waitForFonts = false,
  label,
}: {
  draw: Silhouette
  cols: number
  rows: number
  dot?: number
  gap?: number
  delay?: number
  waitForFonts?: boolean
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

  useEffect(() => {
    const canvas = canvasRef.current!
    const ctx = canvas.getContext('2d')!
    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.round(width * dpr)
    canvas.height = Math.round(height * dpr)
    ctx.scale(dpr, dpr)

    let cancelled = false
    let raf = 0

    const start = () => {
      if (cancelled) return
      const dots = sample(draw, cols, rows)
      const palette = readPalette()

      const drawFrame = (t: number) => {
        ctx.clearRect(0, 0, width, height)
        const epoch = animate ? Math.floor(Math.max(0, t) / GRAIN_EPOCH) : 0
        for (const d of dots) {
          const local = animate ? t - d.appear : 1
          if (local <= 0) continue
          // A slice of settled grain re-rolls its tone every epoch — the
          // crawling-static texture that keeps a dither feeling alive.
          const rerolled = animate && hash(d.seed + epoch * 31) % 34 === 0
          const seed = rerolled ? hash(d.seed ^ Math.imul(epoch, 2654435761)) : d.seed
          ctx.globalAlpha = baseAlpha(d, seed) * (animate ? Math.min(1, local / 0.18) : 1)
          ctx.fillStyle = tone(d, seed, palette)
          ctx.fillRect(d.x * pitch, d.y * pitch, dot, dot)
        }
        ctx.globalAlpha = 1
      }

      if (!animate) {
        drawFrame(Number.MAX_SAFE_INTEGER)
        return
      }
      let lastEpoch = -1
      const t0 = performance.now()
      const frame = (now: number) => {
        const t = (now - t0) / 1000 - delay
        const epoch = Math.floor(Math.max(0, t) / GRAIN_EPOCH)
        // Per-frame drawing only while dissolving in; afterwards the canvas
        // repaints only when the grain epoch ticks (~7fps).
        if (t < BUILD_END || epoch !== lastEpoch) {
          lastEpoch = epoch
          drawFrame(t)
        }
        raf = requestAnimationFrame(frame)
      }
      raf = requestAnimationFrame(frame)
    }

    if (waitForFonts) {
      void document.fonts.ready.then(() => start())
    } else {
      start()
    }
    return () => {
      cancelled = true
      cancelAnimationFrame(raf)
    }
  }, [animate, draw, cols, rows, dot, gap, pitch, width, height, delay, waitForFonts, resolved])

  return <canvas ref={canvasRef} role="img" aria-label={label} aria-hidden={label ? undefined : true} style={{ width, height }} />
}

const drawJaz: Silhouette = (g, w, h) => {
  const family = "600 100px 'Inter Variable', 'Inter', sans-serif"
  g.font = family
  g.textAlign = 'center'
  const probe = g.measureText('jaz')
  const probeHeight = probe.actualBoundingBoxAscent + probe.actualBoundingBoxDescent
  const size = 100 * Math.min((0.9 * w) / probe.width, (0.88 * h) / probeHeight)
  g.font = family.replace('100px', `${size}px`)
  const m = g.measureText('jaz')
  g.fillText('jaz', w / 2, h / 2 + (m.actualBoundingBoxAscent - m.actualBoundingBoxDescent) / 2)
}

// The boot wordmark: "jaz" dissolving in as dithered brand grain.
export function DitherWordmark({ delay = 0 }: { delay?: number }) {
  return <DitherArt draw={drawJaz} cols={112} rows={48} delay={delay} waitForFonts label="jaz" />
}
