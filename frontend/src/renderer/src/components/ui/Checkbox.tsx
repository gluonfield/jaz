import { Check } from 'lucide-react'

// Monochrome checkbox matching the design system. Checked = filled ink box with
// a contrasting tick; unchecked = faint bordered box. Mirrors Switch's props.
export function Checkbox({
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
      role="checkbox"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`grid size-4 shrink-0 cursor-pointer place-items-center rounded-[5px] transition-colors duration-150 disabled:cursor-default disabled:opacity-50 ${
        checked ? 'bg-ink text-bg' : 'bg-transparent text-transparent ring-1 ring-border'
      }`}
    >
      <Check size={11} strokeWidth={3} />
    </button>
  )
}
