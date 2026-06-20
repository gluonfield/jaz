import { MessageSquareQuote } from 'lucide-react'

export function MessageQuotes({ quotes }: { quotes: string[] }) {
  if (quotes.length === 0) return null
  return (
    <div className="mb-1.5 flex flex-wrap justify-end gap-1">
      {quotes.map((text, index) => (
        <div
          key={`${index}-${text.slice(0, 24)}`}
          className="group relative flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
        >
          <MessageSquareQuote size={13} className="shrink-0 text-primary" />
          <span className="text-ink-3">Selection {index + 1}</span>
          <span className="max-w-[200px] truncate text-ink">{text}</span>
          <div className="pointer-events-none absolute bottom-full right-0 z-20 mb-1.5 hidden max-h-[200px] w-max max-w-[360px] overflow-hidden rounded-card bg-ink px-3 py-2 text-xs whitespace-pre-wrap text-bg shadow-[0_8px_30px_rgba(0,0,0,0.22)] group-hover:block">
            {text}
          </div>
        </div>
      ))}
    </div>
  )
}
