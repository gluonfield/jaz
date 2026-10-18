import { useState } from 'react'
import { OverviewRuns } from '@/components/session/OverviewRuns'
import { SidePanelShell } from '@/components/session/SidePanelShell'
import type { ThreadDetailsView } from '@/components/session/ThreadView'
import { Popover } from '@/components/ui/Popover'
import { Segmented } from '@/components/ui/Segmented'
import type { Bot, BotColor } from '@/lib/api/types'
import { BOT_COLORS, BOT_SHAPES } from '@/lib/bots'
import { OVERVIEW_PANEL_WIDTH } from '@/lib/sidePanelTabs'
import { BotAgentSettings } from './BotAgentSettings'
import { BotAvatar } from './BotAvatar'
import { BotNameInput } from './BotNameInput'
import { BotRoutines } from './BotRoutines'
import { useUpdateBot } from './useUpdateBot'

type Tab = 'routines' | 'subtasks' | 'agent'

const TABS: { value: Tab; label: string }[] = [
  { value: 'routines', label: 'Routines' },
  { value: 'subtasks', label: 'Subtasks' },
  { value: 'agent', label: 'Agent' },
]

export function BotDetails({ bot, focusName, agentSession, working, threads, subagents }: { bot: Bot; focusName: boolean } & ThreadDetailsView) {
  const [tab, setTab] = useState<Tab>('routines')
  return (
    <SidePanelShell width={OVERVIEW_PANEL_WIDTH} variant="hug" className="gap-4 px-4 py-4">
      <div className="flex items-center gap-2.5">
        <AvatarEditor bot={bot} />
        <BotNameInput
          key={bot.id}
          bot={bot}
          autoFocus={focusName}
          className="min-w-0 flex-1 rounded-md px-1 text-[15px] font-semibold hover:bg-list-hover focus:bg-list-hover"
        />
      </div>
      <Segmented value={tab} options={TABS} onChange={setTab} layoutId={`bot-details-${bot.id}`} />
      {tab === 'routines' ? (
        <BotRoutines bot={bot} />
      ) : tab === 'subtasks' ? (
        threads.length || subagents.length ? (
          <OverviewRuns threads={threads} subagents={subagents} />
        ) : (
          <p className="text-[13px] text-ink-3">{bot.name} hands longer work to subtasks that run in the background.</p>
        )
      ) : (
        <BotAgentSettings bot={bot} agentSession={agentSession} working={working} />
      )}
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
          className="grid size-10 shrink-0 place-items-center rounded-full transition-transform duration-150 hover:scale-[1.04] active:scale-[0.97]"
        >
          <BotAvatar avatar={avatar} size={34} />
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
