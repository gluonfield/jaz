import { useQuery } from '@tanstack/react-query'
import { botFilesQuery } from '@/lib/api/bots'
import type { Bot, BotFile, BotFileName } from '@/lib/api/types'
import { hasTime, shortDate } from '@/lib/format/time'

const CARDS: { name: BotFileName; about: string }[] = [
  { name: 'AGENTS.md', about: 'Who it is' },
  { name: 'MEMORY.md', about: 'What it knows' },
]

export function BotIdentity({ bot, onOpen }: { bot: Bot; onOpen: (name: BotFileName) => void }) {
  const files = useQuery(botFilesQuery(bot.id)).data
  return (
    <div className="grid grid-cols-2 gap-2">
      {CARDS.map(({ name, about }) => (
        <button
          key={name}
          type="button"
          onClick={() => onOpen(name)}
          className="flex cursor-pointer flex-col items-start gap-0.5 rounded-xl border border-border bg-bg p-3 text-left transition-colors duration-150 hover:bg-list-hover"
        >
          <span className="text-[13px] font-semibold text-ink">{name}</span>
          <span className="text-[12px] text-ink-3">{about}</span>
          <span className="text-[11px] text-ink-3 tabular-nums">{updated(files?.find((file) => file.name === name))}</span>
        </button>
      ))}
    </div>
  )
}

function updated(file?: BotFile): string {
  if (!file) return ''
  return hasTime(file.updated_at) ? `Updated ${shortDate(file.updated_at)}` : 'Not written yet'
}
