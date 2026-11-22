import { type ColorTheme, linkedHues, resetColorTheme, setColorTheme, tintOf, useColorTheme } from '@/lib/appearanceScheme'
import { ColorPad, GrainDial, StrengthWave } from './ColorControls'
import { SettingsCard } from './SettingsCard'

// Named starting points: two neutrals (no colours, so only a trace of the hue
// shows: cool or warm), muted single colours, then the vivid linked ones. None
// of them touch the globe lines.
const THEMES: { name: string; theme: Partial<ColorTheme> }[] = [
  { name: 'Graphite', theme: { hue: 264, colors: 0 } },
  { name: 'Charcoal', theme: { hue: 90, colors: 0 } },
  { name: 'Slate', theme: { hue: 255, colors: 1, saturation: 0.2, strength: 0.25 } },
  { name: 'Navy', theme: { hue: 264, colors: 1, saturation: 0.35, strength: 0.3 } },
  { name: 'Cobalt', theme: { hue: 264, colors: 2, harmony: 1, saturation: 0.6, strength: 0.45 } },
  { name: 'Teal', theme: { hue: 200, colors: 1, saturation: 0.35, strength: 0.3 } },
  { name: 'Forest', theme: { hue: 155, colors: 1, saturation: 0.3, strength: 0.3 } },
  { name: 'Bronze', theme: { hue: 80, colors: 1, saturation: 0.25, strength: 0.3 } },
  { name: 'Burgundy', theme: { hue: 15, colors: 1, saturation: 0.35, strength: 0.3 } },
  { name: 'Plum', theme: { hue: 330, colors: 1, saturation: 0.3, strength: 0.3 } },
  { name: 'Sunset', theme: { hue: 20, colors: 3, harmony: 0, saturation: 0.7, strength: 0.5 } },
  { name: 'Aurora', theme: { hue: 190, colors: 3, harmony: 0, saturation: 0.65, strength: 0.5 } },
]

const isActive = (current: ColorTheme, theme: Partial<ColorTheme>) =>
  (Object.keys(theme) as (keyof ColorTheme)[]).every((key) => current[key] === theme[key])

// A theme's accent, at a lightness that reads in both modes, in Zen's swatch
// shapes: one colour flat, two across, three from the corners.
function themeBackground(theme: ColorTheme) {
  const tint = tintOf(theme)
  const [a, b, c] = (theme.colors ? linkedHues(theme) : [theme.hue]).map(
    (hue) => `oklch(${(0.42 + 0.14 * tint).toFixed(3)} ${Math.max(0.012, 0.17 * tint).toFixed(3)} ${hue})`,
  )
  if (!b) return a
  if (!c) return `linear-gradient(135deg, ${a}, ${b})`
  return `radial-gradient(circle at 0% 0%, ${a}, transparent 100%), radial-gradient(circle at 100% 0%, ${b}, transparent 100%), linear-gradient(to top, ${c} 0%, transparent 60%), ${a}`
}

export function ThemePresets({ className = 'flex justify-between' }: { className?: string }) {
  const current = useColorTheme()
  return (
    <div className={className}>
      {THEMES.map(({ name, theme }) => {
        const active = isActive(current, theme)
        return (
          <button
            key={name}
            type="button"
            aria-label={name}
            aria-pressed={active}
            title={name}
            onClick={() => setColorTheme(theme)}
            style={{ background: themeBackground({ ...current, ...theme }) }}
            className={`size-[26px] shrink-0 cursor-pointer rounded-full transition-transform duration-100 hover:scale-105 active:scale-95 ${
              active ? 'ring-2 ring-ink ring-offset-2 ring-offset-surface' : 'shadow-[0_0_1px_1px_rgb(0_0_0/0.1)]'
            }`}
          />
        )
      })}
    </div>
  )
}

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

      <SettingsCard className="mt-4 grid gap-5 p-3 md:grid-cols-[300px_minmax(0,1fr)]">
        <ColorPad theme={theme} />
        <div className="flex min-w-0 flex-col justify-center gap-7">
          <div>
            <div className="mb-3 flex items-baseline justify-between">
              <p className="text-[13px] font-medium text-ink">Theme</p>
              <p className="text-[12px] text-ink-3">{THEMES.find((t) => isActive(theme, t.theme))?.name ?? 'Custom'}</p>
            </div>
            <ThemePresets className="grid grid-cols-[repeat(6,auto)] justify-between gap-y-3.5" />
          </div>
          <div className="flex items-start justify-between gap-4">
            <div>
              <StrengthWave value={theme.strength} onChange={(strength) => setColorTheme({ strength })} />
              <p className="mt-1 text-center text-[12px] text-ink-3">Strength</p>
            </div>
            <div>
              <GrainDial value={theme.grain} onChange={(grain) => setColorTheme({ grain })} />
              <p className="mt-1 text-center text-[12px] text-ink-3">Globe</p>
            </div>
          </div>
        </div>
      </SettingsCard>
    </>
  )
}
