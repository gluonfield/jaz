import { MessageSquareQuote, SlidersHorizontal, X } from 'lucide-react'
import type { ComposerContext } from '@/lib/sendMessage'
import { contextLabel, contextPreviewText } from '@/lib/sendMessage'

export function ContextChip({
  index,
  context,
  align = 'left',
  onRemove,
}: {
  index: number
  context: ComposerContext
  align?: 'left' | 'right'
  onRemove?: () => void
}) {
  const label = contextLabel(context)
  const text = contextPreviewText(context)
  const Icon = context.type === 'selection' ? MessageSquareQuote : SlidersHorizontal
  return (
    <div
      className={`group relative flex items-center gap-1.5 rounded-full bg-bg py-1.5 pl-3 text-xs text-ink-2 transition-colors hover:bg-surface-2 ${
        onRemove ? 'pr-1.5' : 'pr-3'
      }`}
    >
      <Icon size={13} className="shrink-0 text-ink-3" />
      <span className="text-ink">{label}</span>
      {onRemove ? (
        <button
          type="button"
          className="grid size-4 shrink-0 place-items-center rounded-full text-ink-3 transition-colors hover:bg-ink/10 hover:text-ink"
          aria-label={`Remove ${label.toLowerCase()} ${index + 1}`}
          onClick={onRemove}
        >
          <X size={12} />
        </button>
      ) : null}
      <div
        className={`pointer-events-none absolute bottom-full ${align === 'right' ? 'right-0' : 'left-0'} z-tooltip mb-1.5 hidden max-h-[200px] w-max max-w-[360px] overflow-hidden rounded-card bg-ink px-3 py-2 text-xs whitespace-pre-wrap text-bg shadow-[0_8px_30px_rgba(0,0,0,0.22)] group-hover:block`}
      >
        {text}
      </div>
    </div>
  )
}
