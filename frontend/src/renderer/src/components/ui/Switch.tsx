import { motion } from 'motion/react'

// iOS-style toggle. On = brand-filled track with a contrasting knob;
// off = faint track. No border.
export function Switch({
  checked,
  onChange,
  disabled,
  'aria-label': ariaLabel,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  'aria-label'?: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors duration-150 disabled:cursor-default disabled:opacity-50 ${
        checked ? 'bg-primary' : 'bg-ink/20'
      }`}
    >
      <motion.span
        layout
        transition={{ type: 'spring', stiffness: 500, damping: 34 }}
        className={`absolute top-1/2 size-3.5 -translate-y-1/2 rounded-full ${
          checked ? 'right-1 bg-on-primary' : 'left-1 bg-ink/60'
        }`}
      />
    </button>
  )
}
