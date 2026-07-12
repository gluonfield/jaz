import { useEffect, useRef, useState } from 'react'
import type { ReasoningEffortOption } from '@/lib/api/types'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'

const THUMB = 28

function stopPosition(index: number, count: number): string {
  if (count <= 1) return '50%'
  return `calc(${THUMB / 2}px + ${index / (count - 1)} * (100% - ${THUMB}px))`
}

export function ReasoningEffortSlider({
  options,
  value,
  defaultValue,
  disabled,
  onChange,
}: {
  options: ReasoningEffortOption[]
  value: string
  defaultValue?: string
  disabled?: boolean
  onChange: (effort: string) => void
}) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [previewIndex, setPreviewIndex] = useState<number | null>(null)

  const selected = value || defaultValue || ''
  let index = options.findIndex((option) => option.value === selected)
  if (index < 0) index = Math.floor((options.length - 1) / 2)

  const shown = options[previewIndex ?? index]
  const ultra = isUltraEffort(shown?.value)

  const indexFromPointer = (clientX: number): number => {
    const rect = trackRef.current?.getBoundingClientRect()
    if (!rect || rect.width <= THUMB) return index
    const nx = (clientX - rect.left - THUMB / 2) / (rect.width - THUMB)
    return Math.max(0, Math.min(options.length - 1, Math.round(nx * (options.length - 1))))
  }

  return (
    <div className={disabled ? 'pointer-events-none opacity-60' : ''}>
      <p className="text-[13px] text-ink-3">
        Effort{' '}
        <span className={`font-semibold ${ultra ? 'jaz-gradient' : 'text-ink'}`}>
          {shown?.label ?? 'Default'}
        </span>
      </p>
      <div
        ref={trackRef}
        className="relative mt-1.5 h-7"
        onMouseMove={(e) => setPreviewIndex(indexFromPointer(e.clientX))}
        onMouseLeave={() => setPreviewIndex(null)}
      >
        <div
          className={`absolute inset-0 rounded-[10px] bg-ink/10 ${ultra ? 'effort-ultra-track' : ''}`}
        />
        {options.map((option, i) => (
          <span
            key={option.value}
            className={`absolute top-1/2 size-1 -translate-x-1/2 -translate-y-1/2 rounded-full ${
              isUltraEffort(option.value)
                ? 'bg-primary shadow-[0_0_6px_var(--color-primary)]'
                : 'bg-ink/25'
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
          disabled={disabled}
          onChange={(e) => onChange(options[Number(e.target.value)]?.value ?? '')}
          className={`absolute inset-0 w-full cursor-pointer appearance-none bg-transparent outline-none
            [&::-webkit-slider-runnable-track]:h-7
            [&::-webkit-slider-thumb]:mt-1 [&::-webkit-slider-thumb]:h-5 [&::-webkit-slider-thumb]:w-7
            [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-[6px]
            [&::-webkit-slider-thumb]:transition-[background-color,box-shadow] [&::-webkit-slider-thumb]:duration-150
            [&::-moz-range-thumb]:h-5 [&::-moz-range-thumb]:w-7 [&::-moz-range-thumb]:appearance-none
            [&::-moz-range-thumb]:rounded-[6px] [&::-moz-range-thumb]:border-0 ${
              ultra
                ? `[&::-webkit-slider-thumb]:bg-primary
                   [&::-webkit-slider-thumb]:shadow-[0_1px_3px_rgba(0,0,0,0.35),0_0_12px_var(--color-primary)]
                   [&::-moz-range-thumb]:bg-primary`
                : `[&::-webkit-slider-thumb]:bg-ink/90 hover:[&::-webkit-slider-thumb]:bg-ink
                   [&::-webkit-slider-thumb]:shadow-[0_1px_3px_rgba(0,0,0,0.35)]
                   [&::-moz-range-thumb]:bg-ink/90`
            }`}
        />
      </div>
      <div className="mt-1 flex items-baseline justify-between text-[12px] text-ink-3">
        <span>Faster</span>
        <span>Smarter</span>
      </div>
    </div>
  )
}

function isUltraEffort(value: string | undefined): boolean {
  return value === 'ultra' || value === 'ultracode'
}

const CELL = 5
const PIXEL = 4

type DitherCell = {
  x: number
  y: number
  nx: number
  need: number
  ramp: number
  phase: number
  speed: number
  spark: number
}

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

function ditherPalette(): { base: string[]; bright: string[] } {
  const styles = getComputedStyle(document.documentElement)
  const base: string[] = []
  const bright: string[] = []
  for (let i = 1; i <= 5; i++) {
    const [r, g, b] = cssToRgb(styles.getPropertyValue(`--color-rainbow-${i}`).trim())
    base.push(`rgb(${r} ${g} ${b})`)
    const lift = (c: number) => Math.round(c + (255 - c) * 0.65)
    bright.push(`rgb(${lift(r)} ${lift(g)} ${lift(b)})`)
  }
  return { base, bright }
}

function buildCells(width: number, height: number): DitherCell[] {
  const cols = Math.ceil(width / CELL)
  const rows = Math.max(1, Math.floor(height / CELL))
  const offY = (height - rows * CELL) / 2
  const cells: DitherCell[] = []
  for (let c = 0; c < cols; c++) {
    const nx = (c + 0.5) / cols
    for (let r = 0; r < rows; r++) {
      if (Math.random() > 0.55 + 0.45 * nx) continue
      cells.push({
        x: c * CELL + (CELL - PIXEL) / 2,
        y: offY + r * CELL + (CELL - PIXEL) / 2,
        nx,
        need: (1 - nx) * 0.85 + Math.random() * 0.13,
        ramp: 0.5 + 0.5 * Math.pow(nx, 1.2),
        phase: Math.random() * Math.PI * 2,
        speed: 1 + Math.random() * 2.2,
        spark: 0,
      })
    }
  }
  return cells
}

function UltracodeDither({ active }: { active: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const stateRef = useRef({ active, reduced: false, wave: 0, raf: 0 })
  const reducedMotion = useReducedEffectsMotion()

  useEffect(() => {
    const state = stateRef.current
    state.active = active
    state.reduced = reducedMotion
    const canvas = canvasRef.current
    if (!canvas || state.raf || (!active && state.wave === 0)) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let width = 0
    let height = 0
    let cells: DitherCell[] = []
    let palette = { base: [] as string[], bright: [] as string[] }
    const size = () => {
      const rect = canvas.getBoundingClientRect()
      if (!rect.width || !rect.height) return false
      width = rect.width
      height = rect.height
      const dpr = window.devicePixelRatio || 1
      canvas.width = Math.round(width * dpr)
      canvas.height = Math.round(height * dpr)
      ctx.scale(dpr, dpr)
      cells = buildCells(width, height)
      palette = ditherPalette()
      return true
    }

    const draw = (t: number, dt: number) => {
      ctx.clearRect(0, 0, width, height)
      for (const cell of cells) {
        const front = Math.max(0, Math.min(1, (state.wave * 1.04 - cell.need) * 6))
        if (front <= 0.01) continue
        const flow = state.reduced
          ? 1
          : 0.72 +
            0.2 * Math.sin(cell.nx * 14 + t * 3.4 + cell.phase * 0.5) +
            0.08 * Math.sin(t * cell.speed + cell.phase)
        let pulse = 0
        if (!state.reduced) {
          const s = (((t * 0.55 - cell.nx * 1.15 + cell.phase * 0.03) % 1) + 1) % 1
          pulse = Math.exp(-14 * s * s)
          if (cell.spark > 0) cell.spark = Math.max(0, cell.spark - dt * 2.6)
          else if (Math.random() < dt * (0.015 + 0.1 * cell.nx)) cell.spark = 1
        }
        const hue = (((cell.nx * 1.7 - t * 0.16) % 1) + 1) % 1
        const heat = pulse * 1.4 + cell.spark
        const fill = (heat > 0.55 ? palette.bright : palette.base)[
          Math.min(4, Math.floor(hue * 5))
        ]
        ctx.fillStyle = fill
        ctx.globalAlpha = Math.min(
          1,
          front * cell.ramp * flow * (1 + 1.3 * pulse) + cell.spark * 0.9,
        )
        if (cell.spark > 0.25) {
          const grow = 1.5 * cell.spark
          ctx.shadowColor = fill
          ctx.shadowBlur = 8 * cell.spark
          ctx.fillRect(cell.x - grow, cell.y - grow, PIXEL + grow * 2, PIXEL + grow * 2)
          ctx.shadowBlur = 0
        } else {
          ctx.fillRect(cell.x, cell.y, PIXEL, PIXEL)
        }
      }
      ctx.globalAlpha = 1
    }

    let lastMs = performance.now()
    const frame = (ms: number) => {
      const dt = Math.min(0.05, (ms - lastMs) / 1000)
      lastMs = ms
      if (!width && !size()) {
        state.raf = state.active ? requestAnimationFrame(frame) : 0
        return
      }
      const target = state.active ? 1 : 0
      state.wave = state.reduced
        ? target
        : state.wave + (target - state.wave) * (1 - Math.exp(-dt * 5))
      if (!state.active && state.wave < 0.01) {
        state.wave = 0
        state.raf = 0
        ctx.clearRect(0, 0, width, height)
        return
      }
      draw(ms / 1000, dt)
      state.raf = state.reduced ? 0 : requestAnimationFrame(frame)
    }
    state.raf = requestAnimationFrame(frame)
  }, [active, reducedMotion])

  useEffect(() => {
    const state = stateRef.current
    return () => {
      cancelAnimationFrame(state.raf)
      state.raf = 0
    }
  }, [])

  return <canvas ref={canvasRef} aria-hidden className="pointer-events-none absolute inset-0 size-full" />
}
