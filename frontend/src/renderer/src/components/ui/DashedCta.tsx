import { Plus } from 'lucide-react'
import type { ReactNode } from 'react'

// Dashed "create something" card — the empty-state CTA used by lists and pickers.
export function DashedCta({
  title,
  subtitle,
  onClick,
}: {
  title: string
  subtitle: ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full flex-col items-center gap-2 rounded-card border border-dashed border-border px-6 py-12 text-center transition-colors duration-150 hover:border-primary/50 hover:bg-surface"
    >
      <span className="grid size-10 place-items-center rounded-full bg-surface-2 text-ink-3">
        <Plus size={18} />
      </span>
      <span className="text-[14px] font-medium text-ink">{title}</span>
      <span className="max-w-[42ch] text-[13px] text-ink-3">{subtitle}</span>
    </button>
  )
}
