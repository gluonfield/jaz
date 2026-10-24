import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { linkedHues, resetColorTheme, setColorTheme, useColorTheme } from '@/lib/appearanceScheme'
import { ColorPad, GrainDial, StrengthWave, swatch } from './ColorControls'
import { SettingsCard } from './SettingsCard'

// Starting points carry as many colours as you have (one when there are none),
// linked the same way; the pages step from vivid to soft.
const PALETTE_HUES = [20, 50, 85, 140, 175, 215, 264, 320]
const PALETTE_PAGES = [
  { label: 'Vivid', saturation: 0.8 },
  { label: 'Balanced', saturation: 0.55 },
  { label: 'Soft', saturation: 0.3 },
]

// Zen's swatch shapes: one colour flat, two across, three from the corners.
function ideaBackground(hues: number[], saturation: number) {
  const [a, b, c] = hues.map((hue) => swatch(hue, saturation))
  if (!b) return a
  if (!c) return `linear-gradient(135deg, ${a}, ${b})`
  return `radial-gradient(circle at 0% 0%, ${a}, transparent 100%), radial-gradient(circle at 100% 0%, ${b}, transparent 100%), linear-gradient(to top, ${c} 0%, transparent 60%), ${a}`
}

export function PaletteIdeas({ page = 0, className = 'gap-2' }: { page?: number; className?: string }) {
  const theme = useColorTheme()
  const { label, saturation } = PALETTE_PAGES[page]
  const colors = Math.max(1, theme.colors)
  return (
    <div className={`flex ${className}`}>
      {PALETTE_HUES.map((hue) => {
        const active = theme.colors === colors && theme.hue === hue && theme.saturation === saturation
        return (
          <button
            key={hue}
            type="button"
            aria-label={`${label}, hue ${hue}°`}
            aria-pressed={active}
            onClick={() => setColorTheme({ hue, colors, saturation })}
            style={{ background: ideaBackground(linkedHues({ ...theme, hue, colors }), saturation) }}
            className={`size-[26px] shrink-0 cursor-pointer rounded-full transition-transform duration-100 hover:scale-105 active:scale-95 ${
              active ? 'ring-2 ring-ink ring-offset-2 ring-offset-surface' : 'shadow-[0_0_1px_1px_rgb(0_0_0/0.1)]'
            }`}
          />
        )
      })}
    </div>
  )
}

const PAGE_BUTTON =
  'grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 transition-colors duration-150 hover:text-ink disabled:cursor-default disabled:opacity-30 disabled:hover:text-ink-3'

export function ColorThemeSettings() {
  const theme = useColorTheme()
  const [page, setPage] = useState(0)
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
            <div className="flex items-baseline justify-between">
              <p className="text-[13px] font-medium text-ink">Palette</p>
              <p className="text-[12px] text-ink-3">{PALETTE_PAGES[page].label}</p>
            </div>
            <div className="mt-3 flex items-center gap-1">
              <button
                type="button"
                aria-label="Previous palette"
                disabled={page === 0}
                onClick={() => setPage(page - 1)}
                className={PAGE_BUTTON}
              >
                <ChevronLeft size={16} />
              </button>
              <PaletteIdeas page={page} className="flex-1 justify-between" />
              <button
                type="button"
                aria-label="Next palette"
                disabled={page === PALETTE_PAGES.length - 1}
                onClick={() => setPage(page + 1)}
                className={PAGE_BUTTON}
              >
                <ChevronRight size={16} />
              </button>
            </div>
            <div aria-hidden className="mt-2.5 flex justify-center gap-1.5">
              {PALETTE_PAGES.map(({ label }, index) => (
                <span key={label} className={`size-[5px] rounded-full bg-ink ${index === page ? 'opacity-60' : 'opacity-20'}`} />
              ))}
            </div>
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
