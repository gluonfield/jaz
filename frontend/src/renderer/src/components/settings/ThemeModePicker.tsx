import { type ThemePref, useTheme } from '@/lib/theme'

// A miniature Jaz window in one mode: the theme's wash (the .theme-tile rule in
// globals.css) around a card with a couple of lines and a button. The System
// tile lays a dark one over the right part of a light one.
function Mini({ mode, clip }: { mode: 'light' | 'dark'; clip?: boolean }) {
  const dark = mode === 'dark'
  return (
    <div
      aria-hidden
      data-mode={mode}
      className={`theme-tile absolute inset-0 ${dark ? 'text-white' : 'text-black'}`}
      style={clip ? { clipPath: 'polygon(58% 0, 100% 0, 100% 100%, 42% 100%)' } : undefined}
    >
      <div className="absolute left-2.5 top-3.5 flex flex-col gap-1 opacity-25">
        <span className="size-[7px] rounded-[2px] bg-current" />
        <span className="size-[7px] rounded-[2px] bg-current" />
        <span className="size-[7px] rounded-[2px] bg-current" />
      </div>
      <div
        className={`absolute inset-y-2 left-7 right-2 rounded-lg p-3 shadow-[0_3px_10px_-4px_rgb(0_0_0/0.4)] ${
          dark ? 'bg-[oklch(0.195_0.005_var(--hue))]' : 'bg-[oklch(0.99_0.002_var(--hue))]'
        }`}
      >
        <div className="h-1.5 w-2/5 rounded-full bg-current opacity-45" />
        <div className="mt-1.5 h-1.5 w-3/5 rounded-full bg-current opacity-20" />
        <div className="absolute bottom-2.5 left-3 h-2.5 w-8 rounded bg-primary" />
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
        className={`relative aspect-[2/1] w-full overflow-hidden rounded-xl transition ${
          active ? 'ring-2 ring-primary ring-offset-2 ring-offset-bg' : 'ring-1 ring-border/70 group-hover:ring-border'
        }`}
      >
        <Mini mode={value === 'dark' ? 'dark' : 'light'} />
        {value === 'system' ? <Mini mode="dark" clip /> : null}
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
