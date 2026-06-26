import { type ThemePref, useTheme } from '@/lib/theme'

// A miniature app mock — titlebar bar + a content panel with skeleton lines —
// painted at the given mode's representative colors. The System card stacks a
// light mock under a right-half-clipped dark one for the split look.
type MockColors = { bg: string; panel: string; bar: string }
const LIGHT_MOCK: MockColors = { bg: '#e8eaef', panel: '#ffffff', bar: '#d0d3da' }
const DARK_MOCK: MockColors = { bg: '#26282f', panel: '#1a1b21', bar: '#3b3e47' }

function Mock({ colors, clip }: { colors: MockColors; clip?: boolean }) {
  const bar = (w: string) => <div className={`h-1.5 rounded-full ${w}`} style={{ backgroundColor: colors.bar }} />
  return (
    <div
      aria-hidden
      className="absolute inset-0 flex flex-col gap-2 p-3"
      style={{ backgroundColor: colors.bg, clipPath: clip ? 'inset(0 0 0 50%)' : undefined }}
    >
      <div className="flex justify-center">{bar('w-24')}</div>
      <div className="flex-1 space-y-1.5 rounded-lg p-2.5" style={{ backgroundColor: colors.panel }}>
        {bar('w-1/3')}
        {bar('w-1/2')}
        {bar('w-2/5')}
      </div>
    </div>
  )
}

const THEME_LABELS: Record<ThemePref, string> = { system: 'System', light: 'Light', dark: 'Dark' }

function ThemePreviewCard({ value }: { value: ThemePref }) {
  const { theme, setTheme } = useTheme()
  const active = theme === value
  return (
    <button
      type="button"
      role="radio"
      aria-checked={active}
      aria-label={THEME_LABELS[value]}
      onClick={() => setTheme(value)}
      className="group flex cursor-pointer flex-col items-center gap-2"
    >
      <div
        className={`relative aspect-[16/10] w-full overflow-hidden rounded-xl transition ${
          active ? 'ring-2 ring-primary' : 'ring-1 ring-border/70 group-hover:ring-border'
        }`}
      >
        {value !== 'dark' ? <Mock colors={LIGHT_MOCK} /> : null}
        {value !== 'light' ? <Mock colors={DARK_MOCK} clip={value === 'system'} /> : null}
      </div>
      <span className={`text-[13px] ${active ? 'font-medium text-ink' : 'text-ink-2'}`}>
        {THEME_LABELS[value]}
      </span>
    </button>
  )
}

export function ThemeModePicker() {
  return (
    <div role="radiogroup" aria-label="Theme" className="grid grid-cols-3 gap-3">
      <ThemePreviewCard value="system" />
      <ThemePreviewCard value="light" />
      <ThemePreviewCard value="dark" />
    </div>
  )
}
