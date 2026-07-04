import { useEffect, useRef, useState } from 'react'
import type { ReasoningEffortOption } from '@/lib/api/types'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'

// Width of the native range thumb; dots and pointer math share the rail the
// thumb's center travels, so everything stays aligned at every stop.
const THUMB = 28

function stopPosition(index: number, count: number): string {
  if (count <= 1) return '50%'
  return `calc(${THUMB / 2}px + ${index / (count - 1)} * (100% - ${THUMB}px))`
}

// Discrete effort slider, fastest on the left, smartest on the right.
// Hovering the track previews the stop under the pointer; the ultracode stop
// lights the track with a dither field sweeping in from the right.
export function ReasoningEffortSlider({
  options,
  value,
  defaultValue,
  showDefaultReset,
  onChange,
}: {
  // Concrete stops, ordered fastest → smartest ('' never appears here).
  options: ReasoningEffortOption[]
  // '' inherits the configured default; the thumb then parks on defaultValue.
  value: string
  defaultValue?: string
  showDefaultReset?: boolean
  onChange: (effort: string) => void
}) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [previewIndex, setPreviewIndex] = useState<number | null>(null)

  const selected = value || defaultValue || ''
  let index = options.findIndex((option) => option.value === selected)
  if (index < 0) index = Math.floor((options.length - 1) / 2)

  const shown = options[previewIndex ?? index]
  const ultra = shown?.value === 'ultracode'

  const indexFromPointer = (clientX: number): number => {
    const rect = trackRef.current?.getBoundingClientRect()
    if (!rect || rect.width <= THUMB) return index
    const nx = (clientX - rect.left - THUMB / 2) / (rect.width - THUMB)
    return Math.max(0, Math.min(options.length - 1, Math.round(nx * (options.length - 1))))
  }

  return (
    <div className="px-3 pt-1.5 pb-2.5">
      <div className="flex items-baseline justify-between">
        <p className="text-[13px] text-ink-3">
          Effort{' '}
          <span className={`font-semibold ${ultra ? 'text-primary' : 'text-ink'}`}>
            {shown?.label ?? 'Default'}
          </span>
        </p>
        {showDefaultReset ? (
          <button
            type="button"
            onClick={() => onChange('')}
            className={`rounded-full px-1.5 text-[11px] transition-colors duration-150 ${
              value === '' ? 'text-primary' : 'text-ink-3 hover:text-ink'
            }`}
          >
            Default
          </button>
        ) : null}
      </div>
      <div className="mt-1 mb-1.5 flex items-baseline justify-between text-[12px] text-ink-3">
        <span>Faster</span>
        <span>Smarter</span>
      </div>
      <div
        ref={trackRef}
        className="relative h-8"
        onMouseMove={(e) => setPreviewIndex(indexFromPointer(e.clientX))}
        onMouseLeave={() => setPreviewIndex(null)}
      >
        <div className="absolute inset-0 rounded-[10px] bg-ink/10" />
        {options.map((option, i) => (
          <span
            key={option.value}
            className={`absolute top-1/2 size-1 -translate-x-1/2 -translate-y-1/2 rounded-full ${
              option.value === 'ultracode' ? 'bg-primary' : 'bg-ink/25'
            }`}
            style={{ left: stopPosition(i, options.length) }}
          />
        ))}
        <div className="absolute inset-0 overflow-hidden rounded-[10px]">
          <UltracodeDither active={ultra} />
        </div>
        <input
          type="range"
          min={0}
          max={options.length - 1}
          step={1}
          value={index}
          aria-label="Reasoning effort"
          aria-valuetext={shown?.label}
          onChange={(e) => onChange(options[Number(e.target.value)]?.value ?? '')}
          className="absolute inset-0 w-full cursor-pointer appearance-none bg-transparent outline-none
            [&::-webkit-slider-runnable-track]:h-8
            [&::-webkit-slider-thumb]:mt-1 [&::-webkit-slider-thumb]:h-6 [&::-webkit-slider-thumb]:w-7
            [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-[6px]
            [&::-webkit-slider-thumb]:bg-ink/90 hover:[&::-webkit-slider-thumb]:bg-ink
            [&::-webkit-slider-thumb]:shadow-[0_1px_3px_rgba(0,0,0,0.35)]
            [&::-webkit-slider-thumb]:transition-colors [&::-webkit-slider-thumb]:duration-150
            [&::-moz-range-thumb]:h-6 [&::-moz-range-thumb]:w-7 [&::-moz-range-thumb]:appearance-none
            [&::-moz-range-thumb]:rounded-[6px] [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-ink/90"
        />
      </div>
    </div>
  )
}

/* ---------------- ultracode dither ----------------
 * A grid of ~4px pixels over the track, brightness ramping toward the right.
 * Activation sweeps a wavefront right → left (per-cell jitter keeps the edge
 * dithered, not a hard line); deactivation runs the same front in reverse.
 * The wave chases `active` with an exponential approach, so flipping state
 * mid-sweep retargets smoothly instead of restarting. */

const CELL = 5
const PIXEL = 4

type DitherCell = {
  x: number
  y: number
  nx: number
  need: number // wave level at which this cell lights; grows toward the left
  ramp: number // spatial brightness, dim left → bright right
  color: number
  phase: number
  speed: number
}

// Resolve a CSS color (oklch tokens included) to RGB by letting canvas parse it.
function cssToRgb(css: string): [number, number, number] {
  const scratch = document.createElement('canvas')
  scratch.width = scratch.height = 1
  const ctx = scratch.getContext('2d', { willReadFrequently: true })
  if (!ctx) return [124, 108, 255]
  ctx.fillStyle = css
  ctx.fillRect(0, 0, 1, 1)
  const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
  return [r, g, b]
}

// Four primary-derived shades: negative k darkens toward black, positive
// lightens toward white — dim purple at the field's tail, near-white at the
// bright end by the thumb.
function ditherPalette(): string[] {
  const [r, g, b] = cssToRgb(
    getComputedStyle(document.documentElement).getPropertyValue('--color-primary').trim(),
  )
  const shade = (k: number) => {
    const target = k < 0 ? 0 : 255
    const mix = (c: number) => Math.round(c + (target - c) * Math.abs(k))
    return `rgb(${mix(r)} ${mix(g)} ${mix(b)})`
  }
  return [shade(-0.3), shade(0), shade(0.35), shade(0.8)]
}

function buildCells(width: number, height: number): DitherCell[] {
  const cols = Math.ceil(width / CELL)
  const rows = Math.max(1, Math.floor(height / CELL))
  const offY = (height - rows * CELL) / 2
  const cells: DitherCell[] = []
  for (let c = 0; c < cols; c++) {
    const nx = (c + 0.5) / cols
    for (let r = 0; r < rows; r++) {
      // Density thins toward the left: the field should trail off into dark
      // track, not carpet it edge to edge.
      if (Math.random() > 0.1 + 0.9 * Math.pow(nx, 1.1)) continue
      cells.push({
        x: c * CELL + (CELL - PIXEL) / 2,
        y: offY + r * CELL + (CELL - PIXEL) / 2,
        nx,
        need: (1 - nx) * 0.85 + Math.random() * 0.13,
        ramp: 0.3 + 0.7 * Math.pow(nx, 1.3),
        color: Math.min(3, Math.floor((nx * 0.75 + Math.random() * 0.55) * 4)),
        phase: Math.random() * Math.PI * 2,
        speed: 1 + Math.random() * 2.2,
      })
    }
  }
  return cells
}

function UltracodeDither({ active }: { active: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const stateRef = useRef({ active, wave: 0, raf: 0 })
  const reducedMotion = useReducedEffectsMotion()

  useEffect(() => {
    const state = stateRef.current
    state.active = active
    const canvas = canvasRef.current
    if (!canvas || state.raf) return

    const rect = canvas.getBoundingClientRect()
    if (!rect.width || !rect.height) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.round(rect.width * dpr)
    canvas.height = Math.round(rect.height * dpr)
    ctx.scale(dpr, dpr)
    const cells = buildCells(rect.width, rect.height)
    const palette = ditherPalette()

    const draw = (t: number) => {
      ctx.clearRect(0, 0, rect.width, rect.height)
      for (const cell of cells) {
        const front = Math.max(0, Math.min(1, (state.wave * 1.04 - cell.need) * 6))
        if (front <= 0.01) continue
        const twinkle = reducedMotion ? 1 : 0.72 + 0.28 * Math.sin(t * cell.speed + cell.phase)
        ctx.globalAlpha = front * cell.ramp * twinkle
        ctx.fillStyle = palette[cell.color]
        ctx.fillRect(cell.x, cell.y, PIXEL, PIXEL)
      }
      ctx.globalAlpha = 1
    }

    if (reducedMotion) {
      state.wave = state.active ? 1 : 0
      draw(0)
      return
    }

    let lastMs = performance.now()
    const frame = (ms: number) => {
      const dt = Math.min(0.05, (ms - lastMs) / 1000)
      lastMs = ms
      state.wave += ((state.active ? 1 : 0) - state.wave) * (1 - Math.exp(-dt * 5))
      if (!state.active && state.wave < 0.01) {
        state.wave = 0
        state.raf = 0
        ctx.clearRect(0, 0, rect.width, rect.height)
        return
      }
      draw(ms / 1000)
      state.raf = requestAnimationFrame(frame)
    }
    state.raf = requestAnimationFrame(frame)
  }, [active, reducedMotion])

  useEffect(
    () => () => {
      cancelAnimationFrame(stateRef.current.raf)
    },
    [],
  )

  return <canvas ref={canvasRef} aria-hidden className="pointer-events-none absolute inset-0 size-full" />
}
