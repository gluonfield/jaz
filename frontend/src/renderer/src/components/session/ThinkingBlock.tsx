import { ChevronRight, Clock3, LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import { MessageMarkdown } from './MessageMarkdown'

export function ThinkingBlock({ text, pending = false }: { text: string; pending?: boolean }) {
  const [open, setOpen] = useState(false)
  const trimmed = text.trim()
  if (!trimmed) return null

  return (
    <div className="flex w-full max-w-[var(--prose-max)] flex-col items-start">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="-ml-1.5 inline-flex min-h-8 items-center gap-1.5 rounded-control px-1.5 text-left text-[13px] text-ink-3 transition-colors duration-150 hover:text-ink-2 motion-reduce:transition-none"
      >
        <span>{pending ? 'Thinking' : 'Thought process'}</span>
        {pending ? (
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
        ) : null}
        <ChevronRight
          size={13}
          className={`shrink-0 transition-transform duration-150 motion-reduce:transition-none ${open ? 'rotate-90' : ''}`}
          aria-hidden
        />
      </button>

      <Collapse open={open} className="w-full">
        <div className="relative ml-2 border-l border-border/75 py-1 pl-5 select-text">
          <span className="absolute -left-2.5 top-2 flex size-5 items-center justify-center rounded-full bg-bg text-ink-3">
            <Clock3 size={12} aria-hidden />
          </span>
          <div className="max-h-72 overflow-auto [&_.chat-prose]:text-[13px] [&_.chat-prose]:leading-[1.55] [&_.chat-prose]:text-ink-2">
            <MessageMarkdown text={trimmed} />
          </div>
        </div>
      </Collapse>
    </div>
  )
}
