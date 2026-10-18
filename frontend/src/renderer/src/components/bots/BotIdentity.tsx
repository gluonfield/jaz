import { useQuery } from '@tanstack/react-query'
import { BookOpen, Fingerprint, type LucideIcon } from 'lucide-react'
import { botFilesQuery } from '@/lib/api/bots'
import type { Bot, BotFile, BotFileName } from '@/lib/api/types'
import { hasTime, shortDate } from '@/lib/format/time'

const CARDS: { name: BotFileName; about: string; Icon: LucideIcon }[] = [
  { name: 'AGENTS.md', about: 'Who it is', Icon: Fingerprint },
  { name: 'MEMORY.md', about: 'What it knows', Icon: BookOpen },
]

export function BotIdentity({ bot, onOpen }: { bot: Bot; onOpen: (name: BotFileName) => void }) {
  const files = useQuery(botFilesQuery(bot.id)).data
  return (
    <div className="grid grid-cols-2 gap-2">
      {CARDS.map(({ name, about, Icon }) => (
        <button
          key={name}
          type="button"
          onClick={() => onOpen(name)}
          className="flex cursor-pointer flex-col items-start gap-6 rounded-xl border border-border bg-bg p-3 text-left transition-colors duration-150 hover:bg-list-hover"
        >
          <Icon size={18} className="text-ink-3" aria-hidden />
          <span className="flex flex-col gap-0.5">
            <span className="text-[13px] font-semibold text-ink">{name}</span>
            <span className="text-[12px] text-ink-3">{about}</span>
            <span className="text-[11px] text-ink-3 tabular-nums">{updated(files?.find((file) => file.name === name))}</span>
          </span>
        </button>
      ))}
    </div>
  )
}

function updated(file?: BotFile): string {
  if (!file) return ''
  return hasTime(file.updated_at) ? `Updated ${shortDate(file.updated_at)}` : 'Not written yet'
}
