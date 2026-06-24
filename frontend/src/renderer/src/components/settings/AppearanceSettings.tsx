import { motion, useReducedMotion } from 'motion/react'
import type { ReactNode } from 'react'
import { Switch } from '@/components/ui/Switch'
import { FONT_SCALES, useAppearance } from '@/lib/appearance'
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
  description: string
  children: ReactNode
}) {
  return (
    <div className="grid grid-cols-1 gap-2 border-t border-border px-3 py-3 first:border-t-0 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-ink">{title}</p>
        <p className="mt-0.5 text-[12px] text-ink-3">{description}</p>
      </div>
      <div className="md:justify-self-end">{children}</div>
    </div>
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
      </SettingsCard>
    </section>
  )
}
