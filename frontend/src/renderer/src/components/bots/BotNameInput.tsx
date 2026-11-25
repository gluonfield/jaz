import { useRef, useState } from 'react'
import type { Bot } from '@/lib/api/types'
import { useIsCloudViewer } from '@/lib/cloudSession'
import { useUpdateBot } from './useUpdateBot'

// Blur or Enter saves; Escape puts the old name back. Only an edit in progress
// is held here, so a rename made elsewhere shows at once.
export function BotNameInput({ bot, autoFocus, className, onDone }: {
  bot: Bot
  autoFocus?: boolean
  className: string
  onDone?: () => void
}) {
  const isViewer = useIsCloudViewer()
  const [draft, setDraft] = useState<string | null>(null)
  const cancelled = useRef(false)
  const update = useUpdateBot(bot.id)

  const finish = () => {
    const next = draft?.trim()
    if (!isViewer && !cancelled.current && next && next !== bot.name) update.mutate({ name: next })
    setDraft(null)
    onDone?.()
  }

  return (
    <input
      autoFocus={autoFocus && !isViewer}
      disabled={isViewer}
      readOnly={isViewer}
      value={draft ?? bot.name}
      aria-label="Name"
      placeholder="Name"
      onChange={(e) => setDraft(e.target.value)}
      onFocus={(e) => {
        cancelled.current = false
        e.currentTarget.select()
      }}
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
      }}
      onKeyDown={(e) => {
        e.stopPropagation()
        if (e.key === 'Escape') cancelled.current = true
        if (e.key === 'Enter' || e.key === 'Escape') e.currentTarget.blur()
      }}
      onBlur={finish}
      className={`min-w-0 select-text bg-transparent text-ink outline-none disabled:cursor-default ${className}`}
    />
  )
}
