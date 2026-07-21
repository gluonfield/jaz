import { Clock3, LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { MessageMarkdown } from './MessageMarkdown'

export function ThinkingBlock({ text, pending = false }: { text: string; pending?: boolean }) {
  const [open, setOpen] = useState(false)
  const trimmed = text.trim()
  if (!trimmed) return null

  return (
    <div className="flex w-full max-w-[var(--prose-max)] flex-col items-start">
      <DisclosureTrigger
        label={pending ? 'Thinking' : 'Thought process'}
        open={open}
        onClick={() => setOpen((value) => !value)}
        accessory={pending ? (
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
        ) : undefined}
      />

      <Collapse open={open} className="w-full">
        <div className="relative ml-2 border-l border-border/75 py-1 pl-5 select-text">
          <span className="absolute -left-2.5 top-2 flex size-5 items-center justify-center rounded-full bg-bg text-ink-3">
            <Clock3 size={12} aria-hidden />
          </span>
          <div className="thinking-prose max-h-72 overflow-auto">
            <MessageMarkdown text={trimmed} />
          </div>
        </div>
      </Collapse>
    </div>
  )
}
