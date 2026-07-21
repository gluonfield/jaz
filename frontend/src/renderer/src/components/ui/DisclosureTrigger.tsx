import { ChevronRight } from 'lucide-react'
import type { ReactNode } from 'react'

export function DisclosureTrigger({
  label,
  open,
  onClick,
  accessory,
  disabled = false,
  className = '',
}: {
  label: ReactNode
  open: boolean
  onClick: () => void
  accessory?: ReactNode
  disabled?: boolean
  className?: string
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      aria-expanded={disabled ? undefined : open}
      onClick={onClick}
      className={`-ml-1.5 inline-flex min-h-8 max-w-full items-center gap-1.5 rounded-control px-1.5 text-left text-[13px] text-ink-3 transition-colors duration-150 motion-reduce:transition-none enabled:hover:text-ink-2 disabled:cursor-default ${className}`}
    >
      <span className="min-w-0 truncate">{label}</span>
      {accessory}
      <ChevronRight
        size={13}
        className={`shrink-0 transition-transform duration-150 motion-reduce:transition-none ${disabled ? 'opacity-30' : open ? 'rotate-90' : ''}`}
        aria-hidden
      />
    </button>
  )
}
