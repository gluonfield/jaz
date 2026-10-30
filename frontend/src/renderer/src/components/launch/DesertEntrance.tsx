import { useReducedMotion } from 'motion/react'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { dunes, sun } from '@/components/launch/desert'
import {
  WORDMARK_FONT,
  boot,
  clamp01,
  drawText,
  easeInOut,
  hash,
  readPalette,
  threshold,
  toneFor,
} from '@/components/launch/DitherArt'
import { useEffectsEnabled } from '@/lib/appearance'
import { clientRuntime } from '@/lib/clientRuntime'
import { devPreview } from '@/lib/devPreview'

// Boot splash for the desktop window, in the brand's dithered grain: wind pours
// sand into the dunes, the sun (a moon by night) climbs behind them, and the
// wordmark dissolves in above. Once the backend has answered, every grain flies
// onto a dot of the boot art underneath — the screen's own wordmark and desert
// — while the ground fades, so the splash folds down into the app. Any key or
// click cuts straight to that.
//
// Perf: a grain is painted into an offscreen layer once it stops moving, so a
// frame blits that layer and draws only the grains still in flight.

const PITCH = 4
const DOT = 3
const ALPHA_STEPS = 12
const HOLD = 2.4
const LATEST = 4.5
const SETTLE = 0.2
const STAGGER = 0.35
const FLIGHT = 0.7

// `?entrance` plays the splash in a browser dev build; it otherwise belongs to
// the desktop app's main window only.
const plays =
  clientRuntime.windowKind === 'main' && (clientRuntime.kind === 'electron' || devPreview('entrance') !== null)

// A grain travels a quadratic curve from (fx, fy) through (cx, cy) to its home
// cell (x, y), fading in over the first quarter of the trip.
type Grain = {
  x: number
  y: number
  fx: number
  fy: number
  cx: number
  cy: number
  start: number
  settle: number
  ease: (k: number) => number
  tone: number
  alpha: number
  seed: number
  word: boolean
}

// A dot of the boot art on screen, in splash pixels.
type Target = { x: number; y: number; size: number; tone: number; alpha: number }
type Flight = { grain: Grain; target: Target; delay: number }

const random = (n: number) => hash(n) / 4294967296
const linear = (k: number) => k
const easeOut = (k: number) => 1 - (1 - k) ** 3
const lerp = (a: number, b: number, k: number) => a + (b - a) * k
const byColumn = (a: { x: number; y: number }, b: { x: number; y: number }) => a.x - b.x || a.y - b.y

// The whole scene in grid cells. `horizon` is the far crest line: sky grains
// below it are hidden behind the dunes.
function buildDesert(cols: number, rows: number, dark: boolean) {
  const palette = readPalette()
  const tones = [palette.primary, palette.strong, palette.ink, ...palette.rainbow]
  const tone = (seed: number, ambient: boolean) => tones.indexOf(toneFor(seed, ambient, palette))
  const grains: Grain[] = []
  const add = (x: number, y: number, seed: number, g: Partial<Grain> & Pick<Grain, 'start' | 'settle' | 'alpha'>) =>
    grains.push({ x, y, fx: x, fy: y, cx: x, cy: y, ease: linear, tone: tone(seed, false), seed, word: false, ...g })

  // Sand pours in on a wind from the left, far range first.
  const band = Math.round(rows * 0.52)
  const horizon = dunes(cols, rows - band, (x, row, coverage, range) => {
    const y = band + row
    const seed = hash(x + y * cols)
    if (coverage < threshold(x, y, seed)) return
    const fall = 4 + 26 * random(seed + 1)
    const drift = 0.9 * fall
    const start = 0.2 * range + (0.6 * x) / cols + 0.3 * random(seed + 2)
    add(x, y, seed, {
      fx: x - drift,
      fy: y - fall,
      cx: x - drift / 2,
      cy: y - fall,
      start,
      settle: start + 0.55,
      alpha: (0.7 + 0.15 * range) * (0.85 + 0.15 * random(seed + 3)),
    })
  }).map((crest) => band + crest)

  // The sun rises from behind the far range.
  const radius = Math.min(rows * 0.16, cols * 0.14)
  const sunY = rows * 0.54
  const rise = band + 0.1 * rows - (sunY - 1.5 * radius)
  sun(cols * 0.8, sunY, radius, dark, (x, y, coverage, d) => {
    const seed = hash(x + y * cols + 0x51ed)
    if (x < 0 || x >= cols || y < 0 || y >= horizon[x] || coverage < threshold(x, y, seed)) return
    add(x, y, seed, {
      fy: y + rise,
      cy: y + rise / 2,
      ease: easeOut,
      start: 0.55,
      settle: 2,
      alpha: d <= 1 ? 0.9 : 0.6,
    })
  })

  // Dust in the sky; stars at night.
  for (let x = 0; x < cols; x++) {
    for (let y = 0; y < horizon[x]; y++) {
      const seed = hash(x + y * cols + 0xd057)
      const noise = (seed % 4096) / 4096
      if (noise >= 0.008) continue
      const star = dark && noise < 0.0025
      const start = 0.3 + 0.9 * random(seed + 1)
      const alpha = star ? 0.55 + 0.35 * random(seed + 2) : 0.14
      add(x, y, seed, { tone: tone(seed, !star), start, settle: start + 1.6, alpha })
    }
  }

  // The wordmark dissolves in dot by dot, each settling a cell or two.
  const off = new OffscreenCanvas(cols, rows)
  const g = off.getContext('2d', { willReadFrequently: true })!
  const wordTop = Math.round(rows * 0.07)
  const wordRows = Math.round(rows * 0.22)
  g.fillStyle = '#fff'
  g.translate(0, wordTop)
  drawText('jaz')(g, cols, wordRows)
  const letters = g.getImageData(0, wordTop, cols, wordRows).data
  for (let row = 0; row < wordRows; row++) {
    for (let x = 0; x < cols; x++) {
      const y = row + wordTop
      const seed = hash(x + y * cols + 0x3a2)
      if (letters[(row * cols + x) * 4 + 3] / 255 < threshold(x, y, seed)) continue
      const start = 0.85 + (0.18 * x) / cols + 0.95 * random(seed + 1)
      add(x, y, seed, {
        fy: y - 2,
        cy: y - 1,
        start,
        settle: start + 0.25,
        alpha: 0.85 + 0.15 * random(seed + 2),
        word: true,
      })
    }
  }

  const streaks = Array.from({ length: Math.round(cols * 0.35) }, (_, i) => ({
    x: random(i * 5) * cols,
    y: Math.round(rows * (0.45 + 0.55 * random(i * 5 + 1))),
    speed: 20 + 50 * random(i * 5 + 2),
    length: 2 + Math.floor(4 * random(i * 5 + 3)),
    alpha: 0.15 + 0.25 * random(i * 5 + 4),
    tone: tone(hash(i), false),
  }))

  return { grains: grains.sort((a, b) => a.settle - b.settle), horizon, streaks, tones }
}

// Pairs the splash's grains with the boot art's dots — wordmark grains with
// wordmark dots, the rest with everything else — each side in column order so
// the scene folds down without grains crossing. Returns the canvases it lands
// on, to keep hidden until the grains arrive.
function land(grains: Grain[], frame: DOMRect, tones: string[]) {
  const words: Target[] = []
  const scenery: Target[] = []
  const canvases: HTMLCanvasElement[] = []
  for (const landing of boot.landings) {
    const rect = landing.canvas.getBoundingClientRect()
    if (!rect.width || rect.bottom < frame.top || rect.top > frame.bottom) continue
    const scale = rect.width / landing.width
    const into = landing.key.startsWith('wordmark:') ? words : scenery
    for (const d of landing.dots) {
      into.push({
        x: rect.left - frame.left + d.x * landing.pitch * scale,
        y: rect.top - frame.top + d.y * landing.pitch * scale,
        size: landing.dot * scale,
        tone: tones.indexOf(d.color),
        alpha: d.alpha,
      })
    }
    canvases.push(landing.canvas)
  }
  const pair = (sources: Grain[], targets: Target[]): Flight[] => {
    if (!sources.length) return []
    sources.sort(byColumn)
    return targets.sort(byColumn).map((target, i) => ({
      grain: sources[Math.floor((i * sources.length) / targets.length)],
      target,
      delay: STAGGER * clamp01(target.x / frame.width) + 0.1 * random(i),
    }))
  }
  const wordGrains = grains.filter((g) => g.word)
  const sceneGrains = grains.filter((g) => !g.word)
  return { flights: [...pair(wordGrains, words), ...pair(sceneGrains, scenery)], canvases }
}

// Collects dots by tone and quantized alpha so a frame costs one fill per bucket.
function painter(ctx: CanvasRenderingContext2D | OffscreenCanvasRenderingContext2D, tones: string[]) {
  let paths: Path2D[] = []
  return {
    dot(tone: number, alpha: number, x: number, y: number, size = DOT) {
      if (alpha < 0.03) return
      const key = tone * ALPHA_STEPS + Math.round(Math.min(1, alpha) * (ALPHA_STEPS - 1))
      ;(paths[key] ??= new Path2D()).rect(x, y, size, size)
    },
    flush() {
      paths.forEach((path, key) => {
        ctx.globalAlpha = (key % ALPHA_STEPS) / (ALPHA_STEPS - 1)
        ctx.fillStyle = tones[Math.floor(key / ALPHA_STEPS)]
        ctx.fill(path)
      })
      paths = []
      ctx.globalAlpha = 1
    },
  }
}

export function DesertEntrance({ ready }: { ready: boolean }) {
  const effects = useEffectsEnabled()
  const reduced = useReducedMotion()
  const [done, setDone] = useState(!plays)
  if (done || !effects || reduced) return null
  return <Desert ready={ready} onDone={() => setDone(true)} />
}

function Desert({ ready, onDone }: { ready: boolean; onDone: () => void }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const readyRef = useRef(ready)
  const doneRef = useRef(onDone)
  useEffect(() => {
    readyRef.current = ready
    doneRef.current = onDone
  })

  // Layout effect: the ground is painted, and boot art told to build at once
  // underneath, before the first frame, so the app never flashes through.
  useLayoutEffect(() => {
    const canvas = canvasRef.current!
    const { clientWidth: width, clientHeight: height } = canvas
    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.round(width * dpr)
    canvas.height = Math.round(height * dpr)
    const ctx = canvas.getContext('2d')!
    ctx.scale(dpr, dpr)
    const ground = getComputedStyle(document.documentElement).getPropertyValue('--color-bg').trim()
    ctx.fillStyle = ground
    ctx.fillRect(0, 0, width, height)
    boot.splash = true

    let cancelled = false
    let skipped = false
    let raf = 0
    let veiled: HTMLCanvasElement[] = []
    const skip = () => {
      skipped = true
    }
    const unveil = () => {
      for (const c of veiled) c.style.visibility = ''
      boot.splash = false
    }
    window.addEventListener('pointerdown', skip, true)
    window.addEventListener('keydown', skip, true)

    const run = () => {
      if (cancelled) return
      const cols = Math.ceil(width / PITCH)
      const rows = Math.ceil(height / PITCH)
      const dark = document.documentElement.classList.contains('dark')
      const { grains, horizon, streaks, tones } = buildDesert(cols, rows, dark)

      const layer = new OffscreenCanvas(canvas.width, canvas.height)
      const layerCtx = layer.getContext('2d')!
      layerCtx.scale(dpr, dpr)
      const baker = painter(layerCtx, tones)
      const paint = painter(ctx, tones)

      let baked = 0
      let readyAt: number | null = null
      let revealAt: number | null = null
      let flights: Flight[] = []
      const t0 = performance.now()
      const frame = (now: number) => {
        const t = (now - t0) / 1000
        if (readyRef.current) readyAt ??= t
        // land only once the app has had a moment to mount its boot art
        const landing = readyAt !== null && t >= Math.max(HOLD, readyAt + SETTLE)
        if (revealAt === null && (skipped || t >= LATEST || landing)) {
          revealAt = t
          canvas.style.pointerEvents = 'none'
          ;({ flights, canvases: veiled } = land(grains, canvas.getBoundingClientRect(), tones))
          for (const c of veiled) c.style.visibility = 'hidden'
        }
        const r = revealAt === null ? -1 : t - revealAt

        for (; baked < grains.length && grains[baked].settle <= t; baked++) {
          const g = grains[baked]
          baker.dot(g.tone, g.alpha, g.x * PITCH, g.y * PITCH)
        }
        baker.flush()

        ctx.clearRect(0, 0, width, height)
        ctx.globalAlpha = 1 - clamp01((r - 0.25) / 0.5)
        ctx.fillStyle = ground
        ctx.fillRect(0, 0, width, height)
        // the settled scene thins out as its grains take off
        ctx.globalAlpha = 1 - clamp01(r / 0.35)
        ctx.drawImage(layer, 0, 0, width, height)
        ctx.globalAlpha = 1
        for (let i = baked; i < grains.length && r < 0; i++) {
          const g = grains[i]
          if (g.start > t) continue
          const raw = (t - g.start) / (g.settle - g.start)
          const k = g.ease(raw)
          const u = 1 - k
          const x = u * u * g.fx + 2 * u * k * g.cx + k * k * g.x
          const y = u * u * g.fy + 2 * u * k * g.cy + k * k * g.y
          // sky grains still below the far crests are behind the dunes
          if (g.y < horizon[g.x] && y >= horizon[Math.round(x)]) continue
          paint.dot(g.tone, g.alpha * Math.min(1, raw * 4), Math.round(x) * PITCH, Math.round(y) * PITCH)
        }
        const calm = clamp01((t - 0.2) / 0.6) * (1 - clamp01(r / 0.35))
        for (const s of streaks) {
          const head = Math.round(((s.x + s.speed * t) % (cols + 12)) - 6)
          for (let j = 0; j < s.length; j++) {
            paint.dot(s.tone, s.alpha * calm * (1 - j / s.length), (head - j) * PITCH, s.y * PITCH)
          }
        }
        for (const { grain, target, delay } of flights) {
          const k = easeInOut(clamp01((r - delay) / FLIGHT))
          const tone = k < 0.5 || target.tone < 0 ? grain.tone : target.tone
          const x = lerp(grain.x * PITCH, target.x, k)
          const y = lerp(grain.y * PITCH, target.y, k)
          paint.dot(tone, lerp(grain.alpha, target.alpha, k), x, y, lerp(DOT, target.size, k))
        }
        paint.flush()

        if (r > STAGGER + 0.1 + FLIGHT) {
          unveil()
          doneRef.current()
          return
        }
        raf = requestAnimationFrame(frame)
      }
      raf = requestAnimationFrame(frame)
    }

    // The wordmark is sampled from Inter, so load it before building.
    void document.fonts.load(WORDMARK_FONT).then(run, run)
    return () => {
      cancelled = true
      cancelAnimationFrame(raf)
      unveil()
      window.removeEventListener('pointerdown', skip, true)
      window.removeEventListener('keydown', skip, true)
    }
  }, [])

  return <canvas ref={canvasRef} aria-hidden className="fixed inset-0 z-[120] size-full" />
}
