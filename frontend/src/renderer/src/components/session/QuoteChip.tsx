import { MessageSquareQuote, X } from 'lucide-react'

// A quoted selection rendered as a compact pill: a quote glyph, the word
// "Selection", and the full quoted text on hover. Used both as a removable draft
// chip in the composer and as a read-only chip in sent user messages. `align`
// flips the preview's anchor so it stays on-screen for right-aligned bubbles.
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
    <div
      className={`group relative flex items-center gap-1.5 rounded-full bg-bg py-1.5 pl-3 text-xs text-ink-2 transition-colors hover:bg-surface-2 ${
        onRemove ? 'pr-1.5' : 'pr-3'
      }`}
    >
      <MessageSquareQuote size={13} className="shrink-0 text-ink-3" />
      <span className="text-ink">Selection</span>
      {onRemove ? (
        <button
          type="button"
          className="grid size-4 shrink-0 place-items-center rounded-full text-ink-3 transition-colors hover:bg-ink/10 hover:text-ink"
          aria-label={`Remove selection ${index + 1}`}
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
