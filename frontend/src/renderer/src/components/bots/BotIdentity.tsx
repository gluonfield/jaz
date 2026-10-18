import { useQuery } from '@tanstack/react-query'
import { botFilesQuery } from '@/lib/api/bots'
import type { Bot, BotFile, BotFileName } from '@/lib/api/types'
import { BOT_FILE_LABELS } from '@/lib/bots'
import { hasTime } from '@/lib/format/time'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'

const CARDS: { name: BotFileName; about: string; tone: string }[] = [
  { name: 'AGENTS.md', about: 'Who it is', tone: 'identity-card-soul' },
  { name: 'MEMORY.md', about: 'What it knows', tone: 'identity-card-memory' },
]

// The day a file last changed, as a stamp in the person's own date order.
const STAMP = new Intl.DateTimeFormat(undefined, { day: '2-digit', month: '2-digit', year: '2-digit' })

export function BotIdentity({ bot, onOpen }: { bot: Bot; onOpen: (name: BotFileName) => void }) {
  const files = useQuery(botFilesQuery(bot.id)).data
  const still = useReducedEffectsMotion()
  return (
    <div className="grid grid-cols-2 gap-2.5">
      {CARDS.map(({ name, about, tone }) => (
        <button
          key={name}
          type="button"
          onClick={() => onOpen(name)}
          className={`identity-card ${tone} ${still ? 'identity-card-still' : ''} flex aspect-[1.12] cursor-pointer flex-col justify-between rounded-[22px] p-4 text-left text-white transition-transform duration-200 ease-out hover:scale-[1.02] active:scale-[0.98] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary`}
        >
          <span className="flex flex-col gap-1">
            <span className="text-[17px] leading-none font-semibold tracking-tight uppercase">{BOT_FILE_LABELS[name]}</span>
            <span className="font-mono text-[10px] tracking-[0.04em] text-white/75 uppercase">{about}</span>
          </span>
          <span className="font-mono text-[13px] text-white/85 tabular-nums">{stamp(files?.find((file) => file.name === name))}</span>
        </button>
      ))}
    </div>
  )
}

function stamp(file?: BotFile): string {
  if (!file) return ''
  return hasTime(file.updated_at) ? STAMP.format(new Date(file.updated_at)).replaceAll('/', '.') : 'Not written yet'
}
