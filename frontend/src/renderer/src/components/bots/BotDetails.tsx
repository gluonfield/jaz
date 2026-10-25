import { Fingerprint, List, Pencil, Repeat, SlidersHorizontal } from 'lucide-react'
import { motion } from 'motion/react'
import { useId, useState } from 'react'
import { SidePanelShell } from '@/components/session/SidePanelShell'
import type { ThreadDetailsView } from '@/components/session/ThreadView'
import { Popover } from '@/components/ui/Popover'
import type { Bot, BotColor, BotFileName } from '@/lib/api/types'
import { BOT_COLORS, BOT_SHAPES, botMood } from '@/lib/bots'
import { OVERVIEW_PANEL_WIDTH } from '@/lib/sidePanelTabs'
import { BotActivity } from '@/components/bots/BotActivity'
import { BotAgentSettings } from './BotAgentSettings'
import { BotAvatar } from './BotAvatar'
import { BotFileEditor } from './BotFileEditor'
import { BotIdentity } from './BotIdentity'
import { BotNameInput } from './BotNameInput'
import { BotRoutineModal } from './BotRoutineModal'
import { BotRoutines } from './BotRoutines'
import { useUpdateBot } from './useUpdateBot'

const TABS = { Activity: List, Routines: Repeat, Identity: Fingerprint, Agent: SlidersHorizontal }
const NAMES = Object.keys(TABS) as (keyof typeof TABS)[]

export function BotDetails({ bot, focusName, agentSession, working, subagents }: { bot: Bot; focusName: boolean } & ThreadDetailsView) {
  const [tab, setTab] = useState<keyof typeof TABS>('Activity')
  const id = useId()
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
      <div
        role="tablist"
        aria-label="Bot details"
        className="flex self-center rounded-full bg-bg p-0.5"
        onKeyDown={(event) => {
          const index = NAMES.indexOf(tab)
          const next = { ArrowLeft: index - 1, ArrowRight: index + 1, Home: 0, End: -1 }[event.key]
          if (next === undefined) return
          event.preventDefault()
          const name = NAMES[(next + NAMES.length) % NAMES.length]
          setTab(name)
          document.getElementById(id + name)?.focus()
        }}
      >
        {NAMES.map((name) => {
          const Icon = TABS[name]
          return (
            <motion.button
              key={name}
              id={id + name}
              type="button"
              role="tab"
              aria-label={name}
              aria-selected={name === tab}
              aria-controls={id}
              tabIndex={name === tab ? 0 : -1}
              onClick={() => setTab(name)}
              whileTap={{ scale: 0.96 }}
              className="relative flex h-8 w-10 cursor-pointer items-center justify-center rounded-full text-ink-2 transition-colors duration-150 before:absolute before:inset-x-0 before:-inset-y-1 hover:text-ink aria-selected:text-ink"
            >
              {name === tab ? (
                <motion.span
                  layoutId={id}
                  transition={{ type: 'spring', duration: 0.32, bounce: 0 }}
                  className="absolute inset-0 rounded-full bg-surface-2 shadow-sm ring-1 ring-border/50"
                />
              ) : null}
              <Icon size={16} className="relative" />
            </motion.button>
          )
        })}
      </div>
      <div role="tabpanel" id={id} aria-labelledby={id + tab}>
        {tab === 'Activity' ? (
          <BotActivity bot={bot} subagents={subagents} onOpenRoutine={setRoutine} onOpenFile={setFile} />
        ) : tab === 'Routines' ? (
          <BotRoutines bot={bot} onOpen={setRoutine} />
        ) : tab === 'Identity' ? (
          <BotIdentity bot={bot} onOpen={setFile} />
        ) : (
          <BotAgentSettings bot={bot} agentSession={agentSession} working={working} />
        )}
      </div>
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
