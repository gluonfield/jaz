import { LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { ThinkingDetail } from './ThinkingDetail'

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
        <div className="relative w-full py-0.5 before:absolute before:bottom-4 before:left-[9px] before:top-4 before:w-px before:bg-border/75">
          <ThinkingDetail text={trimmed} />
        </div>
      </Collapse>
    </div>
  )
}
