import { useEffect, useRef } from 'react'
import type { ReasoningEffortOption } from '@/lib/api/types'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'

function stopPosition(index: number, count: number): string {
  if (count <= 1) return '50%'
  return `calc(var(--effort-thumb) / 2 + ${index / (count - 1)} * (100% - var(--effort-thumb)))`
}

export function ReasoningEffortSlider({
  options,
  value,
  defaultValue,
  compact = false,
  disabled,
  onChange,
}: {
  options: ReasoningEffortOption[]
  value: string
  defaultValue?: string
  compact?: boolean
  disabled?: boolean
  onChange: (value: string) => void
}) {
  const selected = value || defaultValue || ''
  const index = options.findIndex((option) => option.value === selected)
  const ultra = isUltraEffort(selected)
  const reducedMotion = useReducedEffectsMotion()
  const position = stopPosition(Math.max(0, index), options.length)
  const transition = reducedMotion ? '' : 'transition-[left,width,opacity,background-color,box-shadow,scale] duration-150 ease-out'

  return (
    <div className={`${compact ? '[--effort-thumb:28px] [--effort-track:24px]' : '[--effort-thumb:32px] [--effort-track:28px]'} ${disabled ? 'opacity-60' : ''}`}>
      {!compact ? (
        <p className="text-[13px] text-ink-3">
          Effort <span className={`font-semibold ${ultra ? 'jaz-gradient' : 'text-ink'}`}>{options[index]?.label ?? 'Default'}</span>
        </p>
      ) : null}
      <div className={`group relative flex items-center ${compact ? 'h-9' : 'h-10'}`}>
        <div className="absolute inset-x-0 h-(--effort-track) overflow-hidden rounded-[10px] bg-ink/10">
          <div
            data-effort-fill
            className={`absolute inset-y-0 left-0 bg-primary ${transition}`}
            style={{ width: position, opacity: index >= 0 && !ultra ? 1 : 0 }}
          />
          <UltracodeDither active={ultra} />
        </div>
        {options.map((option, i) => (
          <span
            key={option.value}
            className={`pointer-events-none absolute top-1/2 size-1 -translate-x-1/2 -translate-y-1/2 rounded-full ${i < index && !ultra ? 'bg-on-primary/50' : 'bg-ink/30'}`}
            style={{ left: stopPosition(i, options.length) }}
          />
        ))}
        <span
          data-effort-thumb
          aria-hidden
          className={`pointer-events-none absolute top-1/2 size-(--effort-thumb) -translate-x-1/2 -translate-y-1/2 rounded-full group-focus-within:brightness-110 ${transition} ${reducedMotion ? '' : 'group-active:scale-105'} ${ultra
            ? 'bg-primary shadow-[0_1px_3px_rgba(0,0,0,0.35),0_0_12px_var(--color-primary)]'
            : 'bg-white shadow-[0_1px_3px_rgba(0,0,0,0.35)]'}`}
          style={{ left: position, opacity: index < 0 ? 0 : 1 }}
        />
        <input
          type="range"
          min={0}
          max={options.length - 1}
          step={1}
          value={Math.max(0, index)}
          aria-label="Reasoning effort"
          aria-valuetext={options[index]?.label ?? (selected || 'Default')}
          disabled={disabled}
          onChange={(event) => onChange(options[Number(event.target.value)].value)}
          onClick={(event) => {
            if (index < 0) {
              onChange(options[Number(event.currentTarget.value)].value)
            }
          }}
          onKeyDown={(event) => {
            if (index < 0 && ['Home', 'ArrowLeft', 'ArrowDown'].includes(event.key)) {
              event.preventDefault()
              onChange(options[0].value)
            }
          }}
          className="absolute inset-0 w-full cursor-pointer appearance-none rounded-full opacity-0 outline-none disabled:cursor-default
            [&::-webkit-slider-runnable-track]:h-(--effort-track)
            [&::-webkit-slider-thumb]:-mt-0.5 [&::-webkit-slider-thumb]:size-(--effort-thumb)
            [&::-webkit-slider-thumb]:appearance-none
            [&::-moz-range-thumb]:size-(--effort-thumb) [&::-moz-range-thumb]:appearance-none
            [&::-moz-range-thumb]:border-0"
        />
      </div>
      {!compact ? (
        <div className="flex justify-between text-[12px] text-ink-3">
          <span>Faster</span>
          <span>Smarter</span>
        </div>
      ) : null}
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
