import { useQuery } from '@tanstack/react-query'
import type { CSSProperties } from 'react'
import { botFilesQuery } from '@/lib/api/bots'
import type { Bot, BotFile, BotFileName } from '@/lib/api/types'
import { BOT_COLORS, BOT_FILE_LABELS } from '@/lib/bots'
import { hasTime, shortDate } from '@/lib/format/time'
import { BotSilhouette } from './BotAvatar'

const CARDS: { name: BotFileName; about: string; tilt: string }[] = [
  { name: 'AGENTS.md', about: 'Who it is', tilt: 'rotate(-8deg)' },
  { name: 'MEMORY.md', about: 'What it knows', tilt: 'scaleX(-1) rotate(-8deg)' },
]

// Both cards wear the bot's own colour, with its shape faded into a corner.
export function BotIdentity({ bot, onOpen }: { bot: Bot; onOpen: (name: BotFileName) => void }) {
  const files = useQuery(botFilesQuery(bot.id)).data
  return (
    <div className="grid grid-cols-2 gap-2" style={{ '--bot': BOT_COLORS[bot.avatar.color] } as CSSProperties}>
      {CARDS.map(({ name, about, tilt }) => (
        <button
          key={name}
          type="button"
          onClick={() => onOpen(name)}
          className="relative flex min-h-[116px] cursor-pointer flex-col items-start gap-0.5 overflow-hidden rounded-xl border border-border bg-[color-mix(in_oklab,var(--bot)_10%,var(--color-bg))] p-3 text-left transition-colors duration-150 hover:bg-[color-mix(in_oklab,var(--bot)_16%,var(--color-bg))]"
        >
          <span className="relative text-[13px] font-semibold text-ink">{BOT_FILE_LABELS[name]}</span>
          <span className="relative text-[12px] text-ink-3">{about}</span>
          <span className="relative mt-1 text-[11px] text-ink-3 tabular-nums">{updated(files?.find((file) => file.name === name))}</span>
          <span aria-hidden className="pointer-events-none absolute -right-7 -bottom-11 opacity-30" style={{ transform: tilt }}>
            <BotSilhouette avatar={bot.avatar} size={96} />
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
