import { motion } from 'motion/react'
import type { ReactNode } from 'react'

// Inline segmented control (the SidePanelControl pattern): a quiet pill track
// where the active option carries a spring-animated lozenge. Sizes to its
// content — never full width. Designed to sit on a `bg-surface` background.
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  layoutId,
  disabled = false,
}: {
  value: T
  options: { value: T; label: string; icon?: ReactNode }[]
  onChange: (value: T) => void
  /** unique id so the active lozenge animates only within this control */
  layoutId: string
  disabled?: boolean
}) {
  return (
    <div className="inline-flex h-8 items-center self-start rounded-full bg-bg p-0.5">
      {options.map((option) => {
        const active = option.value === value
        return (
          <motion.button
            key={option.value}
            type="button"
            aria-pressed={active}
            disabled={disabled}
            onClick={() => onChange(option.value)}
            whileTap={disabled ? undefined : { scale: 0.96 }}
            className={`relative flex h-7 cursor-pointer items-center gap-1.5 rounded-full px-3 text-[13px] font-medium whitespace-nowrap transition-colors duration-150 disabled:cursor-default disabled:opacity-60 ${
              active ? 'text-ink' : 'text-ink-2 hover:text-ink'
            }`}
          >
            {active ? (
              <motion.span
                layoutId={layoutId}
                transition={{ type: 'spring', duration: 0.32, bounce: 0 }}
                className="absolute inset-0 rounded-full bg-surface-2 shadow-sm ring-1 ring-border/50"
              />
            ) : null}
            <span className="relative flex items-center gap-1.5">
              {option.icon}
              {option.label}
            </span>
          </motion.button>
        )
      })}
    </div>
  )
}
