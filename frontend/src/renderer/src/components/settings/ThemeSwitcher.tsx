import { Monitor, Moon, Sun } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { type ThemePref, useTheme } from '@/lib/theme'

const OPTIONS: { value: ThemePref; icon: typeof Monitor; label: string }[] = [
  { value: 'system', icon: Monitor, label: 'System' },
  { value: 'light', icon: Sun, label: 'Light' },
  { value: 'dark', icon: Moon, label: 'Dark' },
]

// Three icon buttons with a single pill that slides between them (shared
// `layoutId`). The pill is the lightest plane in either theme so it always
// reads as the active selection.
export function ThemeSwitcher() {
  const { theme, setTheme } = useTheme()
  const reduceMotion = useReducedMotion()

  return (
    <div
      role="radiogroup"
      aria-label="Theme"
      className="inline-flex items-center gap-1 rounded-full bg-surface-2 p-1 dark:bg-bg"
    >
      {OPTIONS.map(({ value, icon: Icon, label }) => {
        const active = theme === value
        return (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => setTheme(value)}
            className={`relative flex h-8 cursor-pointer items-center gap-1.5 rounded-full px-3 text-[13px] transition-colors duration-150 ${
              active ? 'text-primary' : 'text-ink-3 hover:text-ink'
            }`}
          >
            {active ? (
              <motion.span
                layoutId="theme-pill"
                className="absolute inset-0 rounded-full bg-bg shadow-sm ring-1 ring-border/60 dark:bg-surface-2"
                transition={
                  reduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 500, damping: 38 }
                }
              />
            ) : null}
            <Icon size={15} className="relative shrink-0" />
            <span className="relative">{label}</span>
          </button>
        )
      })}
    </div>
  )
}
