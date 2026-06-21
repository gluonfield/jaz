import { QuoteChip } from './QuoteChip'

export function MessageQuotes({ quotes }: { quotes: string[] }) {
  if (quotes.length === 0) return null
  return (
    <div className="mb-1.5 flex flex-wrap justify-end gap-1">
      {quotes.map((text, index) => (
        <QuoteChip key={`${index}-${text.slice(0, 24)}`} index={index} text={text} align="right" />
      ))}
    </div>
  )
}
