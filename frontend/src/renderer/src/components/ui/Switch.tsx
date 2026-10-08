import { motion } from 'motion/react'
import type { ReactNode } from 'react'

// iOS-style toggle. On = brand-filled track; off = faint track; the knob stays
// white in both.
function SwitchTrack({ checked, compact = false }: { checked: boolean; compact?: boolean }) {
  return (
    <span
      aria-hidden
      className={`relative inline-flex shrink-0 items-center rounded-full transition-colors duration-150 ${
        compact ? 'h-4 w-7' : 'h-5 w-9'
      } ${checked ? 'bg-primary' : 'bg-ink/20'}`}
    >
      <motion.span
        layout
        transition={{ type: 'spring', stiffness: 500, damping: 34 }}
        className={`absolute rounded-full bg-white shadow-[0_1px_2px_rgba(0,0,0,0.3)] ${
          compact ? 'size-3' : 'size-3.5'
        } ${checked ? (compact ? 'right-0.5' : 'right-1') : compact ? 'left-0.5' : 'left-1'}`}
      />
    </span>
  )
}

export function Switch({
  checked,
  onChange,
  disabled,
  'aria-label': ariaLabel,
  className = '',
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  'aria-label'?: string
  className?: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex shrink-0 cursor-pointer rounded-full disabled:cursor-default disabled:opacity-50 ${className}`}
    >
      <SwitchTrack checked={checked} />
    </button>
  )
}

// A menu row that is itself the switch.
export function SwitchRow({
  icon,
  label,
  checked,
  disabled,
  onChange,
}: {
  icon?: ReactNode
  label: string
  checked: boolean
  disabled?: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-label={label}
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 enabled:hover:bg-surface-2 disabled:cursor-default disabled:opacity-50 ${
        checked ? 'text-ink' : 'text-ink-2'
      }`}
    >
      {icon}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <SwitchTrack checked={checked} compact />
    </button>
  )
}
