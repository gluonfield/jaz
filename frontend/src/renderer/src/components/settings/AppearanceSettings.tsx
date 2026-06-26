import { motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useState } from 'react'
import { Switch } from '@/components/ui/Switch'
import { FONT_SCALES, useAppearance } from '@/lib/appearance'
import {
  applyPreset,
  type ModeSchemes,
  resetScheme,
  sameScheme,
  setMode,
  THEME_PRESETS,
  useScheme,
} from '@/lib/appearanceScheme'
import { FontPicker } from './FontPicker'
import { SettingsCard } from './SettingsCard'
import { ThemeSwitcher } from './ThemeSwitcher'

const SIZE_LABELS: Record<number, string> = {
  0.9: 'Small',
  1: 'Default',
  1.1: 'Large',
  1.25: 'Larger',
}

// Segmented control mirroring ThemeSwitcher: a pill slides to the active size.
function FontSizeSwitcher({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  const reduceMotion = useReducedMotion()
  return (
    <div
      role="radiogroup"
      aria-label="Font size"
      className="inline-flex items-center gap-1 rounded-full bg-surface-2 p-1 dark:bg-bg"
    >
      {FONT_SCALES.map((scale) => {
        const active = Math.abs(scale - value) < 0.001
        return (
          <button
            key={scale}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(scale)}
            className={`relative flex h-8 cursor-pointer items-center rounded-full px-3 text-[13px] transition-colors duration-150 ${
              active ? 'text-primary' : 'text-ink-3 hover:text-ink'
            }`}
          >
            {active ? (
              <motion.span
                layoutId="fontsize-pill"
                className="absolute inset-0 rounded-full bg-bg shadow-sm ring-1 ring-border/60 dark:bg-surface-2"
                transition={
                  reduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 500, damping: 38 }
                }
              />
            ) : null}
            <span className="relative">{SIZE_LABELS[scale] ?? scale}</span>
          </button>
        )
      })}
    </div>
  )
}

// One label/control row inside a card. Rows stack with hairline dividers.
function Row({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <div className="grid grid-cols-1 gap-2 border-t border-border px-3 py-3 first:border-t-0 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-ink">{title}</p>
        {description ? <p className="mt-0.5 text-[12px] text-ink-3">{description}</p> : null}
      </div>
      <div className="md:justify-self-end">{children}</div>
    </div>
  )
}

const HEX = /^#[0-9a-fA-F]{6}$/

// A native color well (the swatch opens the OS picker) paired with an editable
// hex field. Typing only commits once it parses as #rrggbb so the parent never
// sees an invalid color mid-edit.
function ColorField({
  title,
  value,
  onChange,
}: {
  title: string
  value: string
  onChange: (hex: string) => void
}) {
  const [text, setText] = useState(value)
  useEffect(() => setText(value), [value])
  const commit = (next: string) => {
    setText(next)
    if (HEX.test(next)) onChange(next.toLowerCase())
  }
  return (
    <Row title={title}>
      <div className="flex w-40 items-center gap-2 rounded-control bg-surface-2 py-1 pl-2.5 pr-1 ring-1 ring-border/60 focus-within:ring-1 focus-within:ring-primary">
        <input
          type="text"
          value={text}
          spellCheck={false}
          aria-label={`${title} hex value`}
          onChange={(e) => commit(e.target.value)}
          className="min-w-0 flex-1 bg-transparent font-mono text-[12px] uppercase text-ink outline-none"
        />
        <span
          className="relative h-5 w-5 shrink-0 overflow-hidden rounded ring-1 ring-border/70"
          style={{ backgroundColor: value }}
        >
          <input
            type="color"
            value={value}
            aria-label={title}
            onChange={(e) => commit(e.target.value)}
            className="absolute -inset-2 cursor-pointer opacity-0"
          />
        </span>
      </div>
    </Row>
  )
}

// Per-mode editor: a preset picker plus the three colors and contrast that every
// other token is derived from (see lib/appearanceScheme.ts).
function ThemeModeCard({ mode }: { mode: keyof ModeSchemes }) {
  const schemes = useScheme()
  const s = schemes[mode]
  const presetId = THEME_PRESETS.find((p) => sameScheme(p[mode], s))?.id ?? 'custom'
  return (
    <SettingsCard className="overflow-hidden">
      <div className="flex items-center justify-between border-b border-border px-3 py-2.5">
        <p className="text-[13px] font-medium text-ink">{mode === 'light' ? 'Light theme' : 'Dark theme'}</p>
        <select
          value={presetId}
          aria-label={`${mode} theme preset`}
          onChange={(e) => {
            const preset = THEME_PRESETS.find((p) => p.id === e.target.value)
            if (preset) applyPreset(mode, preset)
          }}
          className="cursor-pointer rounded-control bg-surface-2 px-2 py-1 text-[13px] text-ink outline-none ring-1 ring-border/60 focus:ring-1 focus:ring-primary"
        >
          {presetId === 'custom' ? (
            <option value="custom" disabled>
              Custom
            </option>
          ) : null}
          {THEME_PRESETS.map((p) => (
            <option key={p.id} value={p.id}>
              {p.label}
            </option>
          ))}
        </select>
      </div>
      <ColorField title="Accent" value={s.accent} onChange={(accent) => setMode(mode, { accent })} />
      <ColorField title="Background" value={s.background} onChange={(background) => setMode(mode, { background })} />
      <ColorField title="Foreground" value={s.foreground} onChange={(foreground) => setMode(mode, { foreground })} />
      <Row title="Contrast" description="How far surfaces and muted text step from the background.">
        <div className="flex items-center gap-3">
          <input
            type="range"
            min={0}
            max={100}
            value={s.contrast}
            aria-label={`${mode} contrast`}
            onChange={(e) => setMode(mode, { contrast: Number(e.target.value) })}
            className="w-40 accent-primary"
          />
          <span className="w-7 text-right font-mono text-[12px] tabular-nums text-ink-2">{s.contrast}</span>
        </div>
      </Row>
    </SettingsCard>
  )
}

export function AppearanceSettings() {
  const { settings, setAppearance } = useAppearance()

  return (
    <section className="py-5">
      <div>
        <p className="text-sm font-medium text-ink">Appearance</p>
        <p className="mt-0.5 text-[13px] text-ink-2">How the interface looks and feels. Defaults match the stock look.</p>
      </div>

      <SettingsCard className="mt-4 overflow-hidden">
        <Row title="Theme" description="Match the system, or pick light or dark.">
          <ThemeSwitcher />
        </Row>
        <Row
          title="Animated effects"
          description="The rainbow glow around the composer, shimmer dots, and particle fields. Off swaps the composer glow for a calm border."
        >
          <Switch
            checked={settings.effects}
            onChange={(value) => setAppearance({ effects: value })}
            aria-label="Animated effects"
          />
        </Row>
        <Row title="Font size" description="Scales the whole interface up or down.">
          <FontSizeSwitcher
            value={settings.fontScale}
            onChange={(value) => setAppearance({ fontScale: value })}
          />
        </Row>
        <Row
          title="Wide layout"
          description="Use more horizontal width for messages, code, and diffs instead of the narrow reading column."
        >
          <Switch
            checked={settings.wideLayout}
            onChange={(value) => setAppearance({ wideLayout: value })}
            aria-label="Wide layout"
          />
        </Row>
      </SettingsCard>

      <div className="mt-8 flex items-end justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-ink">Color theme</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Start from a preset or set the accent, background, and foreground for light and dark
            independently. Every other color is derived from these.
          </p>
        </div>
        <button
          type="button"
          onClick={resetScheme}
          className="shrink-0 cursor-pointer text-[13px] text-ink-3 underline-offset-2 hover:text-ink hover:underline"
        >
          Reset
        </button>
      </div>

      <div className="mt-4 space-y-4">
        <ThemeModeCard mode="light" />
        <ThemeModeCard mode="dark" />
      </div>

      <div className="mt-8">
        <p className="text-sm font-medium text-ink">Fonts</p>
        <p className="mt-0.5 text-[13px] text-ink-2">
          Pick from the fonts installed on your system, or type any family name. Leave blank for the
          defaults.
        </p>
      </div>

      <SettingsCard className="mt-4 overflow-hidden">
        <Row title="Interface font" description="Used for the UI and prose.">
          <FontPicker
            value={settings.uiFont}
            placeholder="Inter"
            ariaLabel="Interface font"
            onChange={(value) => setAppearance({ uiFont: value })}
          />
        </Row>
        <Row title="Monospace font" description="Used for code, diffs, and the editor.">
          <FontPicker
            value={settings.monoFont}
            placeholder="JetBrains Mono"
            ariaLabel="Monospace font"
            monospaceOnly
            onChange={(value) => setAppearance({ monoFont: value })}
          />
        </Row>
      </SettingsCard>

      <div className="mt-8">
        <p className="text-sm font-medium text-ink">Transcript</p>
        <p className="mt-0.5 text-[13px] text-ink-2">How an agent&apos;s work shows up in the conversation.</p>
      </div>

      <SettingsCard className="mt-4 overflow-hidden">
        <Row
          title="Inline agent diffs"
          description="Show file edits as expanded red/green diffs in the conversation instead of collapsed under a menu."
        >
          <Switch
            checked={settings.inlineDiffs}
            onChange={(value) => setAppearance({ inlineDiffs: value })}
            aria-label="Inline agent diffs"
          />
        </Row>
        <Row
          title="Inline shell commands"
          description="Show shell commands the agent runs — the command and its output — expanded in the conversation instead of collapsed under a menu."
        >
          <Switch
            checked={settings.inlineShellCommands}
            onChange={(value) => setAppearance({ inlineShellCommands: value })}
            aria-label="Inline shell commands"
          />
        </Row>
      </SettingsCard>
    </section>
  )
}
