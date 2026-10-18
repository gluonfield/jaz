import { AnimatePresence, motion } from 'motion/react'
import type { ReactNode } from 'react'

const spring = { type: 'spring', duration: 0.32, bounce: 0 } as const

// Inline segmented control (the SidePanelControl pattern): a quiet pill track
// where the active option carries a spring-animated lozenge. Sizes to its
// content — never full width. Designed to sit on a `bg-surface` background.
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  layoutId,
  disabled = false,
  compact = false,
  iconOnly = false,
}: {
  value: T
  options: { value: T; label: string; icon?: ReactNode }[]
  onChange: (value: T) => void
  /** unique id so the active lozenge animates only within this control */
  layoutId: string
  disabled?: boolean
  /** inactive options with an icon show only the icon */
  compact?: boolean
  /** every option shows only its icon, its label left to hover and assistive tech */
  iconOnly?: boolean
}) {
  return (
    <motion.div layout={compact} transition={spring} className="inline-flex h-8 items-center self-start rounded-full bg-bg p-0.5">
      {options.map((option) => {
        const active = option.value === value
        const labelled = !iconOnly && (active || !compact || !option.icon)
        return (
          <motion.button
            key={option.value}
            type="button"
            aria-pressed={active}
            aria-label={labelled ? undefined : option.label}
            title={labelled ? undefined : option.label}
            disabled={disabled}
            onClick={() => onChange(option.value)}
            whileTap={disabled ? undefined : { scale: 0.96 }}
            className={`relative flex h-7 cursor-pointer items-center justify-center gap-1.5 rounded-full text-[13px] font-medium whitespace-nowrap transition-colors duration-150 disabled:cursor-default disabled:opacity-60 ${
              labelled ? 'px-3' : iconOnly ? 'w-12' : 'w-7'
            } ${active ? 'text-ink' : 'text-ink-2 hover:text-ink'}`}
          >
            {active ? (
              <motion.span
                layoutId={layoutId}
                transition={spring}
                className="absolute inset-0 rounded-full bg-surface-2 shadow-sm ring-1 ring-border/50"
              />
            ) : null}
            <motion.span layout={compact && 'position'} transition={spring} className="relative flex items-center gap-1.5">
              {option.icon}
              <AnimatePresence initial={false} mode="popLayout">
                {labelled ? (
                  <motion.span key="label" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.15 }}>
                    {option.label}
                  </motion.span>
                ) : null}
              </AnimatePresence>
            </motion.span>
          </motion.button>
        )
      })}
    </motion.div>
  )
}
