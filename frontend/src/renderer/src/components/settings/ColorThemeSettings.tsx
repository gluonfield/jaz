import { Minus, Plus } from 'lucide-react'
import type { KeyboardEvent, PointerEvent } from 'react'
import {
  applyPreset,
  type ColorTheme,
  linkedHues,
  MAX_COLORS,
  presetMatches,
  resetColorTheme,
  setColorTheme,
  THEME_PRESETS,
  useColorTheme,
} from '@/lib/appearanceScheme'
import { SettingsCard, SettingsRow } from './SettingsCard'

const WHEEL = 176
// Distance from the centre, in wheel pixels: muted near the middle, full at the rim.
const INNER = 24
const SPAN = 58
const radius = (strength: number) => INNER + strength * SPAN
const clampStrength = (strength: number) => Math.min(1, Math.max(0.1, strength))

// Hues at one lightness per mode (--wheel-l on the card), so they read like the wash.
const swatch = (hue: number, chroma: number) => `oklch(var(--wheel-l) ${chroma} ${hue})`
const WHEEL_BACKGROUND = `radial-gradient(circle, var(--color-surface), transparent 72%), conic-gradient(${Array.from(
  { length: 13 },
  (_, i) => swatch(i * 30, 0.13),
).join(', ')})`

const KEY_STEPS: Record<string, [hue: number, strength: number]> = {
  ArrowLeft: [-5, 0],
  ArrowRight: [5, 0],
  ArrowDown: [0, -0.05],
  ArrowUp: [0, 0.05],
}

// The main colour sits where you point: its angle is the hue, its distance from
// the centre the strength. The linked colours follow it round.
function ColorWheel({ theme }: { theme: ColorTheme }) {
  const hues = linkedHues(theme.hue, theme.colors)
  const points = hues.map((hue) => {
    const angle = ((hue - 90) * Math.PI) / 180
    return { x: WHEEL / 2 + Math.cos(angle) * radius(theme.strength), y: WHEEL / 2 + Math.sin(angle) * radius(theme.strength) }
  })
  const pick = (event: PointerEvent<HTMLDivElement>) => {
    const box = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - box.left - box.width / 2
    const y = event.clientY - box.top - box.height / 2
    setColorTheme({
      hue: Math.round((Math.atan2(y, x) * 180) / Math.PI + 450) % 360,
      colors: Math.max(1, theme.colors),
      strength: clampStrength(((Math.hypot(x, y) * WHEEL) / box.width - INNER) / SPAN),
    })
  }
  const nudge = (event: KeyboardEvent) => {
    const step = KEY_STEPS[event.key]
    if (!step) return
    event.preventDefault()
    setColorTheme({
      hue: (theme.hue + step[0] + 360) % 360,
      colors: Math.max(1, theme.colors),
      strength: clampStrength(theme.strength + step[1]),
    })
  }
  return (
    <div
      role="slider"
      tabIndex={0}
      aria-label="Window color"
      aria-valuemin={0}
      aria-valuemax={359}
      aria-valuenow={theme.hue}
      aria-valuetext={theme.colors ? `Hue ${theme.hue}°, strength ${Math.round(theme.strength * 100)}%` : 'Graphite'}
      onPointerDown={(event) => {
        event.currentTarget.setPointerCapture(event.pointerId)
        pick(event)
      }}
      onPointerMove={(event) => {
        if (event.currentTarget.hasPointerCapture(event.pointerId)) pick(event)
      }}
      onKeyDown={nudge}
      style={{ width: WHEEL, height: WHEEL, background: WHEEL_BACKGROUND }}
      className="relative shrink-0 cursor-crosshair touch-none rounded-full shadow-[inset_0_0_0_1px_rgb(0_0_0/0.08),0_6px_18px_-8px_rgb(0_0_0/0.4)]"
    >
      <svg aria-hidden className="absolute inset-0" width={WHEEL} height={WHEEL}>
        {points.slice(1).map((point, index) => (
          <line
            key={hues[index + 1]}
            x1={points[0].x}
            y1={points[0].y}
            x2={point.x}
            y2={point.y}
            stroke="white"
            strokeOpacity={0.85}
            strokeWidth={1.5}
          />
        ))}
      </svg>
      {points.map((point, index) => {
        const size = index ? 15 : 21
        return (
          <span
            key={hues[index]}
            aria-hidden
            className="absolute rounded-full shadow-[0_0_0_2.5px_white,0_2px_6px_rgb(0_0_0/0.35)]"
            style={{
              left: point.x - size / 2,
              top: point.y - size / 2,
              width: size,
              height: size,
              background: swatch(hues[index], 0.04 + theme.strength * 0.12),
            }}
          />
        )
      })}
    </div>
  )
}

function presetBackground(hue: number, colors: number) {
  const hues = linkedHues(hue, colors)
  const stops = hues.length
    ? [...hues, hues[hues.length - 1] - 18].map((h) => swatch(h, 0.13))
    : ['oklch(0.82 0 0)', 'oklch(0.42 0 0)']
  return `linear-gradient(135deg, ${stops.join(', ')})`
}

const STEP_BUTTON =
  'grid size-7 cursor-pointer place-items-center rounded-[7px] text-ink-2 transition-colors duration-150 hover:bg-bg hover:text-ink disabled:cursor-default disabled:opacity-40 disabled:hover:bg-transparent'

export function ColorThemeSettings() {
  const theme = useColorTheme()
  return (
    <>
      <div className="mt-6 flex items-end justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-ink">Color</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Tints the window around your threads. Text, cards and buttons follow from it, so they always match.
          </p>
        </div>
        <button
          type="button"
          onClick={resetColorTheme}
          className="shrink-0 cursor-pointer text-[13px] text-ink-3 underline-offset-2 hover:text-ink hover:underline"
        >
          Reset
        </button>
      </div>

      <SettingsCard className="mt-4 grid overflow-hidden [--wheel-l:0.8] md:grid-cols-[208px_minmax(0,1fr)] dark:[--wheel-l:0.56]">
        <div className="grid place-items-center border-border p-4 max-md:border-b md:border-r">
          <ColorWheel theme={theme} />
        </div>
        <div className="grid md:grid-rows-3">
          <SettingsRow title="Palette" description="Tuned starting points.">
            <div className="flex gap-1.5">
              {THEME_PRESETS.map((preset) => {
                const active = presetMatches(preset, theme)
                return (
                  <button
                    key={preset.id}
                    type="button"
                    aria-label={preset.label}
                    aria-pressed={active}
                    title={preset.label}
                    onClick={() => applyPreset(preset)}
                    style={{ background: presetBackground(preset.hue, preset.colors) }}
                    className={`size-[22px] cursor-pointer rounded-full ${
                      active ? 'ring-2 ring-ink ring-offset-2 ring-offset-surface' : 'ring-1 ring-inset ring-black/10'
                    }`}
                  />
                )
              })}
            </div>
          </SettingsRow>
          <SettingsRow title="Colors" description="Linked: move one and the others turn with it.">
            <div className="flex items-center gap-0.5 rounded-[9px] bg-surface-2 p-0.5">
              <button
                type="button"
                aria-label="Fewer colors"
                disabled={theme.colors === 0}
                onClick={() => setColorTheme({ colors: theme.colors - 1 })}
                className={STEP_BUTTON}
              >
                <Minus size={14} />
              </button>
              <span className="w-5 text-center text-[13px] font-medium tabular-nums text-ink">{theme.colors}</span>
              <button
                type="button"
                aria-label="More colors"
                disabled={theme.colors === MAX_COLORS}
                onClick={() => setColorTheme({ colors: theme.colors + 1 })}
                className={STEP_BUTTON}
              >
                <Plus size={14} />
              </button>
            </div>
          </SettingsRow>
          <SettingsRow title="Grain" description="A fine film texture over the window color.">
            <input
              type="range"
              min={0}
              max={100}
              step={5}
              value={theme.grain}
              aria-label="Grain"
              onChange={(event) => setColorTheme({ grain: Number(event.target.value) })}
              className="w-40 accent-primary"
            />
          </SettingsRow>
        </div>
      </SettingsCard>
    </>
  )
}
