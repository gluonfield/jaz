import { Clock3 } from 'lucide-react'
import { MessageMarkdown } from './MessageMarkdown'

export function ThinkingDetail({ text }: { text: string }) {
  const trimmed = text.trim()
  if (!trimmed) return null
  return (
    <div className="relative min-w-0 py-1 pl-7 select-text">
      <span className="absolute left-0 top-2 flex size-5 items-center justify-center rounded-full bg-bg text-ink-3">
        <Clock3 size={12} aria-hidden />
      </span>
      <div className="thinking-prose max-h-72 overflow-auto">
        <MessageMarkdown text={trimmed} />
      </div>
    </div>
  )
}
