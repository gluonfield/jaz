import { Fingerprint, List, ListTodo, Pencil, Repeat, SlidersHorizontal } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { SidePanelShell } from '@/components/session/SidePanelShell'
import type { ThreadDetailsView } from '@/components/session/ThreadView'
import { Popover } from '@/components/ui/Popover'
import { Segmented } from '@/components/ui/Segmented'
import type { Bot, BotColor, BotFileName } from '@/lib/api/types'
import { BOT_COLORS, BOT_SHAPES, botMood } from '@/lib/bots'
import { OVERVIEW_PANEL_WIDTH } from '@/lib/sidePanelTabs'
import { BotActivity, BotTasks } from '@/components/bots/BotActivity'
import { BotAgentSettings } from './BotAgentSettings'
import { BotAvatar } from './BotAvatar'
import { BotFileEditor } from './BotFileEditor'
import { BotIdentity } from './BotIdentity'
import { BotNameInput } from './BotNameInput'
import { BotRoutineModal } from './BotRoutineModal'
import { BotRoutines } from './BotRoutines'
import { useUpdateBot } from './useUpdateBot'

type Tab = 'tasks' | 'activity' | 'routines' | 'identity' | 'agent'

const TABS: { value: Tab; label: string; icon: ReactNode }[] = [
  { value: 'tasks', label: 'Tasks', icon: <ListTodo size={16} /> },
  { value: 'activity', label: 'Activity', icon: <List size={16} /> },
  { value: 'routines', label: 'Routines', icon: <Repeat size={16} /> },
  { value: 'identity', label: 'Identity', icon: <Fingerprint size={16} /> },
  { value: 'agent', label: 'Agent', icon: <SlidersHorizontal size={16} /> },
]

export function BotDetails({ bot, focusName, agentSession, working, subagents }: { bot: Bot; focusName: boolean } & ThreadDetailsView) {
  const [tab, setTab] = useState<Tab>('tasks')
  const [routine, setRoutine] = useState<string | null>(null)
  const [file, setFile] = useState<BotFileName | null>(null)
  return (
    <SidePanelShell width={OVERVIEW_PANEL_WIDTH} variant="hug" className="gap-5 px-4 pt-6 pb-4">
      <header className="flex flex-col items-center gap-1 text-center">
        <AvatarEditor bot={bot} />
        <BotNameInput
          key={bot.id}
          bot={bot}
          autoFocus={focusName}
          className="mt-2 w-full rounded-md px-1 text-center text-[16px] font-semibold hover:bg-list-hover focus:bg-list-hover"
        />
      </header>
      <div className="flex justify-center [&>div]:h-11 [&_button]:h-10 [&_button]:min-w-10 [&_button]:px-1.5">
        <Segmented value={tab} options={TABS} onChange={setTab} layoutId={`bot-details-${bot.id}`} compact />
      </div>
      {tab === 'tasks' ? (
        <BotTasks bot={bot} />
      ) : tab === 'activity' ? (
        <BotActivity bot={bot} subagents={subagents} onOpenRoutine={setRoutine} onOpenFile={setFile} />
      ) : tab === 'routines' ? (
        <BotRoutines bot={bot} onOpen={setRoutine} />
      ) : tab === 'identity' ? (
        <BotIdentity bot={bot} onOpen={setFile} />
      ) : (
        <BotAgentSettings bot={bot} agentSession={agentSession} working={working} />
      )}
      {routine ? <BotRoutineModal bot={bot} loopId={routine} onClose={() => setRoutine(null)} /> : null}
      {file ? <BotFileEditor bot={bot} name={file} onClose={() => setFile(null)} /> : null}
    </SidePanelShell>
  )
}

// Shapes wear the current color and colors the current shape, so every pick
// previews exactly what it sets.
function AvatarEditor({ bot }: { bot: Bot }) {
  const [open, setOpen] = useState(false)
  const update = useUpdateBot(bot.id)
  const { avatar } = bot
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="below"
      trigger={
        <button
          type="button"
          aria-label="Change face"
          title="Change face"
          onClick={() => setOpen((value) => !value)}
          className="group relative grid size-[72px] cursor-pointer place-items-center rounded-full transition-transform duration-150 hover:scale-[1.03] active:scale-[0.97]"
        >
          <BotAvatar avatar={avatar} size={64} mood={botMood(bot)} live />
          <span className="absolute right-0 bottom-0 grid size-6 place-items-center rounded-full border border-border bg-surface text-ink-2 shadow-sm transition-colors duration-150 group-hover:text-ink">
            <Pencil size={12} aria-hidden />
          </span>
        </button>
      }
    >
      <div className="flex w-60 flex-col gap-2 p-1">
        <div className="grid grid-cols-4 gap-1">
          {BOT_SHAPES.map((shape) => (
            <button
              key={shape}
              type="button"
              aria-label={shape}
              aria-pressed={shape === avatar.shape}
              onClick={() => update.mutate({ avatar: { ...avatar, shape } })}
              className={`grid h-12 place-items-center rounded-[8px] transition-colors duration-150 hover:bg-list-hover ${
                shape === avatar.shape ? 'bg-list-active' : ''
              }`}
            >
              <BotAvatar avatar={{ ...avatar, shape }} size={34} />
            </button>
          ))}
        </div>
        <div className="grid grid-cols-6 gap-1 border-t border-border pt-2">
          {(Object.keys(BOT_COLORS) as BotColor[]).map((color) => (
            <button
              key={color}
              type="button"
              aria-label={color}
              aria-pressed={color === avatar.color}
              onClick={() => update.mutate({ avatar: { ...avatar, color } })}
              className={`grid h-9 place-items-center rounded-[8px] transition-colors duration-150 hover:bg-list-hover ${
                color === avatar.color ? 'bg-list-active' : ''
              }`}
            >
              <BotAvatar avatar={{ ...avatar, color }} size={24} />
            </button>
          ))}
        </div>
      </div>
    </Popover>
  )
}
