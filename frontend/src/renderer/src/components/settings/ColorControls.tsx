import { Minus, MoonStar, Plus, Shuffle, Sparkles, Sun } from 'lucide-react'
import { type KeyboardEvent, type PointerEvent, useId } from 'react'
import grainUrl from '@/assets/grain.png'
import {
  type ColorTheme,
  HARMONIES,
  harmonyOf,
  linkedHues,
  MAX_COLORS,
  setColorTheme,
  washChroma,
} from '@/lib/appearanceScheme'
import { type ThemePref, useTheme } from '@/lib/theme'

// The colour pickers, after Zen's (Arc's): a dotted pad, a wavy strength
// slider and a notched grain dial.

// A colour at the picker's lightness for the mode (--swatch-l in globals.css).
export const swatch = (hue: number, saturation: number) =>
  `oklch(var(--swatch-l) ${washChroma(saturation).toFixed(3)} ${hue})`

const clamp = (value: number) => Math.min(1, Math.max(0, value))

// Drag from inside the control and keep following outside it.
function track(pick: (event: PointerEvent<HTMLDivElement>) => void) {
  return {
    onPointerDown: (event: PointerEvent<HTMLDivElement>) => {
      event.currentTarget.setPointerCapture(event.pointerId)
      pick(event)
    },
    onPointerMove: (event: PointerEvent<HTMLDivElement>) => {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) pick(event)
    },
  }
}

const MODES: { value: ThemePref; label: string; Icon: typeof Sun }[] = [
  { value: 'system', label: 'Automatic', Icon: Sparkles },
  { value: 'light', label: 'Light', Icon: Sun },
  { value: 'dark', label: 'Dark', Icon: MoonStar },
]

const PAD_BUTTON =
  'grid size-[30px] cursor-pointer place-items-center rounded-lg text-black/70 transition-colors duration-150 hover:bg-black/10 disabled:cursor-default disabled:opacity-50 disabled:hover:bg-transparent dark:text-white/90 dark:hover:bg-white/10'

// How far out the dots reach, as a share of the pad's half-width.
const REACH = 0.86

const PAD_KEYS: Record<string, [hue: number, saturation: number]> = {
  ArrowLeft: [-5, 0],
  ArrowRight: [5, 0],
  ArrowDown: [0, -0.05],
  ArrowUp: [0, 0.05],
}

// The main dot sits where you point: its angle is the hue, its distance from
// the centre the saturation. The linked dots turn with it.
export function ColorPad({ theme }: { theme: ColorTheme }) {
  const { theme: mode, setTheme } = useTheme()
  const hues = linkedHues(theme)
  const pick = (event: PointerEvent<HTMLDivElement>) => {
    const box = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - box.left - box.width / 2
    const y = event.clientY - box.top - box.height / 2
    setColorTheme({
      hue: Math.round((Math.atan2(y, x) * 180) / Math.PI + 450) % 360,
      saturation: clamp(Math.hypot(x, y) / ((box.width / 2) * REACH)),
      colors: Math.max(1, theme.colors),
    })
  }
  const nudge = (event: KeyboardEvent) => {
    const step = PAD_KEYS[event.key]
    if (!step) return
    event.preventDefault()
    setColorTheme({
      hue: (theme.hue + step[0] + 360) % 360,
      saturation: clamp(theme.saturation + step[1]),
      colors: Math.max(1, theme.colors),
    })
  }

  return (
    <div className="relative aspect-square w-full max-w-[300px] overflow-hidden rounded-xl bg-black/5 bg-[radial-gradient(rgb(0_0_0/0.1)_1px,transparent_0)] bg-[length:6px_6px] bg-[position:-23px_-23px] dark:bg-white/[0.03] dark:bg-[radial-gradient(rgb(255_255_255/0.07)_1px,transparent_0)]">
      <div
        role="slider"
        tabIndex={0}
        aria-label="Window color"
        aria-valuemin={0}
        aria-valuemax={359}
        aria-valuenow={theme.hue}
        aria-valuetext={theme.colors ? `Hue ${theme.hue}°, saturation ${Math.round(theme.saturation * 100)}%` : 'Graphite'}
        {...track(pick)}
        onKeyDown={nudge}
        className="absolute inset-0 cursor-crosshair touch-none"
      >
        {hues.map((hue, index) => {
          const angle = ((hue - 90) * Math.PI) / 180
          const distance = theme.saturation * REACH * 50
          return (
            <span
              key={hue}
              aria-hidden
              className={`pointer-events-none absolute -translate-x-1/2 -translate-y-1/2 rounded-full border-white shadow-[0_2px_10px_rgb(0_0_0/0.22)] ${
                index ? 'z-[1] size-[22px] border-[3px]' : 'z-[2] size-[38px] border-[6px]'
              }`}
              style={{
                left: `${50 + Math.cos(angle) * distance}%`,
                top: `${50 + Math.sin(angle) * distance}%`,
                background: swatch(hue, theme.saturation),
              }}
            />
          )
        })}
        {hues.length ? null : (
          <span className="pointer-events-none absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 whitespace-nowrap text-[12px] font-semibold text-ink-3">
            Click to add a color
          </span>
        )}
      </div>

      <div role="radiogroup" aria-label="Theme" className="absolute left-1/2 top-3 flex -translate-x-1/2 gap-[5px]">
        {MODES.map(({ value, label, Icon }) => (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={mode === value}
            aria-label={label}
            title={label}
            onClick={() => setTheme(value)}
            className={`${PAD_BUTTON} ${mode === value ? 'bg-black/10 dark:bg-white/10' : ''}`}
          >
            <Icon size={17} strokeWidth={1.75} />
          </button>
        ))}
      </div>

      <div className="absolute bottom-3 left-1/2 flex -translate-x-1/2 gap-[5px]">
        <button
          type="button"
          aria-label="Remove a color"
          title="Remove a color"
          disabled={theme.colors === 0}
          onClick={() => setColorTheme({ colors: theme.colors - 1, harmony: 0 })}
          className={PAD_BUTTON}
        >
          <Minus size={16} />
        </button>
        <button
          type="button"
          aria-label="Add a color"
          title="Add a color"
          disabled={theme.colors === MAX_COLORS}
          onClick={() => setColorTheme({ colors: theme.colors + 1, harmony: 0 })}
          className={PAD_BUTTON}
        >
          <Plus size={16} />
        </button>
        <button
          type="button"
          aria-label={theme.colors > 1 ? `Linked: ${harmonyOf(theme).label}. Change the link` : 'Change the link'}
          title={theme.colors > 1 ? `Linked: ${harmonyOf(theme).label}` : undefined}
          disabled={theme.colors < 2}
          onClick={() => setColorTheme({ harmony: (theme.harmony + 1) % HARMONIES[theme.colors].length })}
          className={PAD_BUTTON}
        >
          <Shuffle size={15} />
        </button>
      </div>
    </div>
  )
}

const WAVE = { width: 224, height: 64, start: 14, end: 210, middle: 32 }

// A sine line that flattens as the value drops, solid up to the thumb and
// faded past it; the thumb grows with the value.
export function StrengthWave({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  const gradient = `wave${useId().replace(/[^\w]/g, '')}`
  const span = WAVE.end - WAVE.start
  let path = ''
  for (let x = WAVE.start; x <= WAVE.end; x += 2) {
    const y = WAVE.middle + Math.sin(((x - WAVE.start) / span) * Math.PI * 9) * 11 * value
    path += `${x === WAVE.start ? 'M' : ' L'}${x} ${y.toFixed(2)}`
  }
  const thumb = { width: 10 + value * 15, height: 40 + value * 15 }
  const pick = (event: PointerEvent<HTMLDivElement>) => {
    const box = event.currentTarget.getBoundingClientRect()
    onChange(clamp((((event.clientX - box.left) / box.width) * WAVE.width - WAVE.start) / span))
  }
  return (
    <div
      role="slider"
      tabIndex={0}
      aria-label="Strength"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(value * 100)}
      {...track(pick)}
      onKeyDown={(event) => {
        const step = { ArrowLeft: -0.05, ArrowDown: -0.05, ArrowRight: 0.05, ArrowUp: 0.05 }[event.key]
        if (step === undefined) return
        event.preventDefault()
        onChange(clamp(value + step))
      }}
      style={{ width: WAVE.width, height: WAVE.height }}
      className="relative cursor-pointer touch-none text-[rgb(90_90_90)] dark:text-[rgb(161_161_161)]"
    >
      <span aria-hidden className="absolute inset-y-5 left-2 right-0 rounded-full bg-black/10 dark:bg-white/10" />
      <svg aria-hidden className="absolute inset-0 overflow-visible" width={WAVE.width} height={WAVE.height}>
        <defs>
          <linearGradient id={gradient} gradientUnits="userSpaceOnUse" x1={WAVE.start} x2={WAVE.end} y1={0} y2={0}>
            <stop offset={0} stopColor="currentColor" />
            <stop offset={value} stopColor="currentColor" />
            <stop offset={value} stopColor="currentColor" stopOpacity={0.5} />
            <stop offset={1} stopColor="currentColor" stopOpacity={0.5} />
          </linearGradient>
        </defs>
        <path d={path} fill="none" stroke={`url(#${gradient})`} strokeWidth={6} strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span
        aria-hidden
        className="pointer-events-none absolute rounded-full bg-black shadow-[0_2px_6px_rgb(0_0_0/0.25)] dark:bg-white"
        style={{
          left: WAVE.start + value * span - thumb.width / 2,
          top: WAVE.middle - thumb.height / 2,
          width: thumb.width,
          height: thumb.height,
        }}
      />
    </div>
  )
}

const DIAL = 64
const RING = 28
const NOTCHES = 16

// Sixteen notches round a face that shows the grain itself; the handle sits
// at the chosen notch, counted clockwise from the top.
export function GrainDial({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  const point = (turn: number, inset = 0) => {
    const angle = turn * Math.PI * 2 - Math.PI / 2
    return { left: DIAL / 2 + Math.cos(angle) * RING - inset, top: DIAL / 2 + Math.sin(angle) * RING - inset }
  }
  const pick = (event: PointerEvent<HTMLDivElement>) => {
    const box = event.currentTarget.getBoundingClientRect()
    const turn = Math.atan2(event.clientY - box.top - box.height / 2, event.clientX - box.left - box.width / 2) / (Math.PI * 2) + 0.25
    onChange((Math.round((((turn % 1) + 1) % 1) * NOTCHES) % NOTCHES) / NOTCHES)
  }
  const handle = point(value)
  return (
    <div
      role="slider"
      tabIndex={0}
      aria-label="Grain"
      aria-valuemin={0}
      aria-valuemax={NOTCHES - 1}
      aria-valuenow={Math.round(value * NOTCHES)}
      {...track(pick)}
      onKeyDown={(event) => {
        const step = { ArrowLeft: -1, ArrowDown: -1, ArrowRight: 1, ArrowUp: 1 }[event.key]
        if (step === undefined) return
        event.preventDefault()
        onChange(Math.min(NOTCHES - 1, Math.max(0, Math.round(value * NOTCHES) + step)) / NOTCHES)
      }}
      style={{ width: DIAL, height: DIAL }}
      className="relative shrink-0 cursor-pointer touch-none"
    >
      {Array.from({ length: NOTCHES }, (_, notch) => (
        <span
          key={notch}
          aria-hidden
          className={`absolute size-[3px] rounded-full bg-black/30 dark:bg-white/30 ${value > 0 && notch / NOTCHES <= value ? '' : 'opacity-40'}`}
          style={point(notch / NOTCHES, 1.5)}
        />
      ))}
      <span
        aria-hidden
        className="absolute inset-[13px] rounded-full mix-blend-hard-light"
        style={{ backgroundImage: `url(${grainUrl})`, opacity: value }}
      />
      <span
        aria-hidden
        className="absolute inset-[13px] rounded-full border border-border/50 bg-[linear-gradient(-45deg,transparent_-10%,rgb(0_0_0/0.1)_110%)] dark:bg-[linear-gradient(-45deg,transparent_-10%,rgb(255_255_255/0.1)_110%)]"
      />
      <span
        aria-hidden
        className="absolute h-[9px] w-1 rounded-[2px] bg-[#757575] dark:bg-[#d1d1d1]"
        style={{ left: handle.left - 2, top: handle.top - 4.5, transform: `rotate(${value * 360}deg)` }}
      />
    </div>
  )
}
