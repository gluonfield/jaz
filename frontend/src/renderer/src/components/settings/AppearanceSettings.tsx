import { motion, useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { Switch } from '@/components/ui/Switch'
import { FONT_SCALES, useAppearance } from '@/lib/appearance'
import { isHomeLogoUrl } from '@/lib/homeWordmark'
import { ColorThemeSettings } from './ColorThemeSettings'
import { FontPicker } from './FontPicker'
import { SettingsCard, SettingsRow as Row } from './SettingsCard'

const SIZE_LABELS: Record<number, string> = {
  0.9: 'Small',
  1: 'Default',
  1.1: 'Large',
  1.25: 'Larger',
}

// Segmented control: a pill slides to the active size.
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

export function AppearanceSettings() {
  const { settings, setAppearance } = useAppearance()
  const [homeWordmark, setHomeWordmark] = useState(settings.homeWordmark)
  useEffect(() => setHomeWordmark(settings.homeWordmark), [settings.homeWordmark])

  return (
    <section className="py-4">
      <p className="text-sm font-medium text-ink">Appearance</p>

      <SettingsCard className="mt-4 overflow-hidden">
        <Row
          title="Animated effects"
          description="The glow around the composer, shimmer dots and particle fields. Off swaps the glow for a calm border."
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
          description="More width for messages, code and diffs instead of the narrow reading column."
        >
          <Switch
            checked={settings.wideLayout}
            onChange={(value) => setAppearance({ wideLayout: value })}
            aria-label="Wide layout"
          />
        </Row>
        <Row title="Model icons" description="Agent and model marks beside threads.">
          <Switch
            checked={settings.showModelIcons}
            onChange={(value) => setAppearance({ showModelIcons: value })}
            aria-label="Model icons"
          />
        </Row>
        <Row
          title="Logo"
          description="A name for dithered text, or a direct image URL. Leave blank for “jaz”."
        >
          <input
            type="text"
            value={homeWordmark}
            placeholder="jaz"
            aria-label="Logo"
            onChange={(event) => {
              setHomeWordmark(event.currentTarget.value)
              setAppearance({ homeWordmark: event.currentTarget.value })
            }}
            className="h-8 w-72 rounded-control bg-surface-2 px-2.5 text-[13px] text-ink outline-none ring-1 ring-border/60 placeholder:text-ink-3 focus:ring-primary"
          />
        </Row>
        {isHomeLogoUrl(settings.homeWordmark) ? (
          <>
            <Row title="Invert image in light mode" description="For white logos on transparent backgrounds.">
              <Switch
                checked={settings.invertHomeLogoInLightMode}
                onChange={(value) => setAppearance({ invertHomeLogoInLightMode: value })}
                aria-label="Invert image in light mode"
              />
            </Row>
            <Row title="Invert image in dark mode" description="For black logos on transparent backgrounds.">
              <Switch
                checked={settings.invertHomeLogoInDarkMode}
                onChange={(value) => setAppearance({ invertHomeLogoInDarkMode: value })}
                aria-label="Invert image in dark mode"
              />
            </Row>
          </>
        ) : null}
      </SettingsCard>

      <ColorThemeSettings />

      <div className="mt-6">
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

      <div className="mt-6">
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
