import { ScrollText } from 'lucide-react'
import { useState } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'

export function ThinkingBlock({ text, pending = false, findActive = false }: {
  text: string
  pending?: boolean
  findActive?: boolean
}) {
  const [open, setOpen] = useState(false)
  const effectiveOpen = open || findActive
  const trimmed = text.trim()
  if (!trimmed) return null

  return (
    <div className="flex w-full max-w-[var(--prose-max)] flex-col items-start">
      <DisclosureTrigger
        label={(
          <span className="flex min-w-0 items-center gap-2">
            <ScrollText className="size-3.5 shrink-0" aria-hidden />
            <span className={pending ? 'live-shimmer' : undefined}>{pending ? 'Thinking' : 'Thought'}</span>
          </span>
        )}
        open={effectiveOpen}
        onClick={() => setOpen((value) => !value)}
      />

      <Collapse open={effectiveOpen} className="w-full">
        <div className="thinking-prose min-w-0 border-l border-border/75 py-1 pl-4 select-text">
          <MessageMarkdown text={trimmed} />
        </div>
      </Collapse>
    </div>
  )
}
