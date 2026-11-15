import { type ColorTheme, linkedHues, resetColorTheme, setColorTheme, useColorTheme } from '@/lib/appearanceScheme'
import { ColorPad, GrainDial, StrengthWave, swatch } from './ColorControls'
import { SettingsCard } from './SettingsCard'

// Named starting points. Each sets only the fields listed, so Graphite keeps
// the hue to come back to and none of them touch the grain.
const THEMES: { name: string; theme: Partial<ColorTheme> }[] = [
  { name: 'Cobalt', theme: { hue: 264, colors: 2, harmony: 1, saturation: 0.6, strength: 0.45 } },
  { name: 'Graphite', theme: { colors: 0 } },
  { name: 'Slate', theme: { hue: 250, colors: 1, harmony: 0, saturation: 0.15, strength: 0.4 } },
  { name: 'Ocean', theme: { hue: 225, colors: 2, harmony: 1, saturation: 0.5, strength: 0.45 } },
  { name: 'Forest', theme: { hue: 160, colors: 1, harmony: 0, saturation: 0.35, strength: 0.4 } },
  { name: 'Sage', theme: { hue: 140, colors: 2, harmony: 1, saturation: 0.2, strength: 0.4 } },
  { name: 'Sand', theme: { hue: 80, colors: 1, harmony: 0, saturation: 0.25, strength: 0.4 } },
  { name: 'Clay', theme: { hue: 50, colors: 2, harmony: 1, saturation: 0.4, strength: 0.4 } },
  { name: 'Rose', theme: { hue: 10, colors: 1, harmony: 0, saturation: 0.35, strength: 0.4 } },
  { name: 'Plum', theme: { hue: 320, colors: 2, harmony: 1, saturation: 0.4, strength: 0.4 } },
  { name: 'Sunset', theme: { hue: 20, colors: 3, harmony: 0, saturation: 0.7, strength: 0.5 } },
  { name: 'Aurora', theme: { hue: 190, colors: 3, harmony: 0, saturation: 0.65, strength: 0.5 } },
]

// Zen's swatch shapes: one colour flat, two across, three from the corners.
function themeBackground(theme: ColorTheme) {
  const [a = 'oklch(var(--swatch-l) 0 0)', b, c] = linkedHues(theme).map((hue) => swatch(hue, theme.saturation))
  if (!b) return a
  if (!c) return `linear-gradient(135deg, ${a}, ${b})`
  return `radial-gradient(circle at 0% 0%, ${a}, transparent 100%), radial-gradient(circle at 100% 0%, ${b}, transparent 100%), linear-gradient(to top, ${c} 0%, transparent 60%), ${a}`
}

// Round swatches in a row, or named tiles in a grid.
export function ThemePresets({ named = false }: { named?: boolean }) {
  const current = useColorTheme()
  return (
    <div className={named ? 'grid grid-cols-4 gap-x-2 gap-y-3' : 'flex justify-between'}>
      {THEMES.map(({ name, theme }) => {
        const active = (Object.keys(theme) as (keyof ColorTheme)[]).every((key) => current[key] === theme[key])
        return (
          <button
            key={name}
            type="button"
            aria-label={name}
            aria-pressed={active}
            title={named ? undefined : name}
            onClick={() => setColorTheme(theme)}
            className={`group flex cursor-pointer flex-col gap-1.5 text-[12px] ${active ? 'font-medium text-ink' : 'text-ink-2 hover:text-ink'}`}
          >
            <span
              style={{ background: themeBackground({ ...current, ...theme }) }}
              className={`transition-transform duration-100 group-hover:scale-[1.03] group-active:scale-[0.97] ${
                named ? 'h-9 w-full rounded-lg' : 'size-[26px] rounded-full'
              } ${active ? 'ring-2 ring-ink ring-offset-2 ring-offset-surface' : 'shadow-[0_0_1px_1px_rgb(0_0_0/0.1)]'}`}
            />
            {named ? name : null}
          </button>
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
        <div className="flex min-w-0 flex-col justify-center gap-6">
          <div>
            <p className="mb-3 text-[13px] font-medium text-ink">Theme</p>
            <ThemePresets named />
          </div>
          <div className="flex items-start justify-between gap-4">
            <div>
              <StrengthWave value={theme.strength} onChange={(strength) => setColorTheme({ strength })} />
              <p className="mt-1 text-center text-[12px] text-ink-3">Strength</p>
            </div>
            <div>
              <GrainDial value={theme.grain} onChange={(grain) => setColorTheme({ grain })} />
              <p className="mt-1 text-center text-[12px] text-ink-3">Grain</p>
            </div>
          </div>
        </div>
      </SettingsCard>
    </>
  )
}
