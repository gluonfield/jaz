import { Plus, X } from 'lucide-react'
import { useState } from 'react'
import { SectionHeader } from '@/components/session/OverviewRuns'
import { SidePanelShell } from '@/components/session/SidePanelShell'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import type { Bot } from '@/lib/api/types'
import { botAvatars } from '@/lib/bots'
import { BotAvatar, BotIcon } from './BotAvatar'
import { BotNameInput } from './BotNameInput'
import { useUpdateBot } from './useUpdateBot'

export function GroupDetails({ group, bots }: { group: Bot; bots: Bot[] }) {
  const update = useUpdateBot(group.id)
  const [adding, setAdding] = useState(false)
  const ids = group.members ?? []
  const members = ids.flatMap((id) => bots.find((bot) => bot.id === id) ?? [])
  const candidates = bots.filter((bot) => bot.kind === 'bot' && !ids.includes(bot.id))

  return (
    <SidePanelShell scroll className="gap-6 px-4 py-5">
      <div className="flex flex-col items-center gap-2">
        <BotIcon avatars={botAvatars(group, bots)} size={72} />
        <BotNameInput
          key={group.id}
          bot={group}
          className="w-full rounded-md px-1 text-center text-[17px] font-semibold hover:bg-list-hover focus:bg-list-hover"
        />
      </div>
      <section className="flex flex-col gap-1">
        <SectionHeader>Members</SectionHeader>
        {members.map((bot) => (
          <div key={bot.id} className="flex h-9 items-center gap-2.5">
            <BotAvatar avatar={bot.avatar} size={24} />
            <span className="min-w-0 flex-1 truncate text-[13px] text-ink">{bot.name}</span>
            <IconButton
              size="xs"
              variant="danger"
              aria-label={`Remove ${bot.name}`}
              title={members.length > 2 ? `Remove ${bot.name}` : 'A group needs two bots'}
              disabled={members.length <= 2}
              onClick={() => update.mutate({ members: ids.filter((id) => id !== bot.id) })}
            >
              <X size={13} />
            </IconButton>
          </div>
        ))}
        {candidates.length ? (
          <Popover
            open={adding}
            onClose={() => setAdding(false)}
            placement="below"
            trigger={
              <button
                type="button"
                onClick={() => setAdding((open) => !open)}
                className="flex h-9 w-full items-center gap-2.5 text-[13px] text-ink-2 transition-colors duration-150 hover:text-ink"
              >
                <span className="grid size-6 place-items-center rounded-full bg-surface-2">
                  <Plus size={13} />
                </span>
                Add bot
              </button>
            }
          >
            {candidates.map((bot) => (
              <MenuRow
                key={bot.id}
                onClick={() => {
                  setAdding(false)
                  update.mutate({ members: [...ids, bot.id] })
                }}
              >
                <span className="flex items-center gap-2">
                  <BotAvatar avatar={bot.avatar} size={16} />
                  {bot.name}
                </span>
              </MenuRow>
            ))}
          </Popover>
        ) : null}
      </section>
    </SidePanelShell>
  )
}
