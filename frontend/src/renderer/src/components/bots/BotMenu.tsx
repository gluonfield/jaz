import { useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from '@tanstack/react-router'
import { Pencil, Pin, Trash2 } from 'lucide-react'
import { ContextMenu, MenuRow } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { deleteBot } from '@/lib/api/bots'
import type { Bot } from '@/lib/api/types'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import { usePins } from './usePins'

// Right-click / press-and-hold actions for a bot or group in the panel.
export function BotMenu({ bot, point, onClose, onRename }: {
  bot: Bot
  point: { x: number; y: number }
  onClose: () => void
  onRename: () => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const openBotId = useParams({ strict: false }).botId
  const toast = useToast()
  const pin = usePins()

  const run = (action: () => void) => () => {
    onClose()
    action()
  }
  const remove = () => {
    if (!window.confirm(bot.kind === 'bot' ? `Delete ${bot.name}? Its routines stop.` : `Delete ${bot.name}?`)) return
    void deleteBot(bot.id)
      .then(() => {
        queryClient.setQueryData<Bot[]>(keys.bots, (bots) => bots?.filter((item) => item.id !== bot.id))
        invalidateSessionLists(queryClient)
        queryClient.invalidateQueries({ queryKey: keys.loops })
        if (openBotId === bot.id) void navigate({ to: '/bots' })
      })
      .catch((error: Error) => toast(`Couldn't delete: ${error.message}`, 'danger'))
  }

  return (
    <ContextMenu point={point} onClose={onClose}>
      <MenuRow onClick={run(() => pin((pins) => (bot.pinned ? pins.filter((id) => id !== bot.id) : [...pins, bot.id])))}>
        <span className="flex items-center gap-2">
          <Pin size={13} className={bot.pinned ? 'fill-current' : ''} />
          {bot.pinned ? 'Unpin' : 'Pin'}
        </span>
      </MenuRow>
      <MenuRow onClick={run(onRename)}>
        <span className="flex items-center gap-2">
          <Pencil size={13} />
          Rename
        </span>
      </MenuRow>
      {bot.main ? null : (
        <MenuRow onClick={run(remove)}>
          <span className="flex items-center gap-2 text-danger">
            <Trash2 size={13} />
            Delete
          </span>
        </MenuRow>
      )}
    </ContextMenu>
  )
}
