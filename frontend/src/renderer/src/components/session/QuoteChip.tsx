import { MessageSquareQuote, X } from 'lucide-react'

// A quoted selection rendered as a pill: label, truncated text, and a full-text
// preview on hover. Used both as a removable draft chip in the composer and as
// a read-only chip in sent user messages. `align` flips the preview's anchor so
// it stays on-screen for right-aligned message bubbles.
export function QuoteChip({
  index,
  text,
  align = 'left',
  onRemove,
}: {
  index: number
  text: string
  align?: 'left' | 'right'
  onRemove?: () => void
}) {
  return (
    <div className="group relative flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2">
      <MessageSquareQuote size={13} className="shrink-0 text-primary" />
      <span className="text-ink-3">Selection {index + 1}</span>
      <span className="max-w-[200px] truncate text-ink">{text}</span>
      {onRemove ? (
        <button
          type="button"
          className="ml-0.5 rounded-full p-0.5 text-ink-3 transition-colors hover:bg-surface hover:text-ink"
          aria-label={`Remove selection ${index + 1}`}
          onClick={onRemove}
        >
          <X size={12} />
        </button>
      ) : null}
      <div
        className={`pointer-events-none absolute bottom-full ${align === 'right' ? 'right-0' : 'left-0'} z-20 mb-1.5 hidden max-h-[200px] w-max max-w-[360px] overflow-hidden rounded-card bg-ink px-3 py-2 text-xs whitespace-pre-wrap text-bg shadow-[0_8px_30px_rgba(0,0,0,0.22)] group-hover:block`}
      >
        {text}
      </div>
    </div>
  )
}
