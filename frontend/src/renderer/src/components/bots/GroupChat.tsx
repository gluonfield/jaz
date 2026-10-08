import { useQuery, useQueryClient } from '@tanstack/react-query'
import { motion, useReducedMotion } from 'motion/react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { ComposerCard } from '@/components/session/Composer'
import { SidePanelControl } from '@/components/session/SidePanelControl'
import { panelSpring } from '@/components/session/SidePanelDrawer'
import { THREAD_COLUMN_CLASS } from '@/components/session/threadLayout'
import { useThreadAutoScroll } from '@/components/session/useThreadAutoScroll'
import { EmptyState } from '@/components/ui/EmptyState'
import { FileDropScope } from '@/components/ui/FileDrop'
import { useToast } from '@/components/ui/toast'
import { botsQuery, sendGroupMessage } from '@/lib/api/bots'
import { markThreadSeen } from '@/lib/api/feed'
import { sessionEventsQuery, uploadSessionAttachment } from '@/lib/api/sessions'
import type { Bot } from '@/lib/api/types'
import { botAvatars, botChat } from '@/lib/bots'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { useSessionHistory } from '@/lib/hooks/useSessionHistory'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'
import { coalesceSessionEvents } from '@/lib/sessionEvents'
import { OVERVIEW_PANEL_WIDTH } from '@/lib/sidePanelTabs'
import { useTitlebarActions, useTitlebarSlot } from '@/lib/titlebar'
import { BotPill } from './BotAvatar'
import { ChatLog } from './ChatLog'
import { GroupDetails } from './GroupDetails'

// A group is a room, not an agent thread: its transcript is the room_message
// events on the group's thread, and messages go to the room, which decides
// which members answer.
export function GroupChat({ group, bots }: { group: Bot; bots: Bot[] }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const reportHistoryError = useCallback(
    (message: string) => toast(`Couldn't load earlier history: ${message}`, 'danger'),
    [toast],
  )
  const history = useSessionHistory(group.id, reportHistoryError)
  const live = useQuery(sessionEventsQuery(group.id))
  useSessionEvents(group.id, history.data?.latest_event_seq)
  const { scrollRef, attachScroll, onScroll, pinToBottom } = useThreadAutoScroll({ resetKey: group.id })
  const [detailsOpen, setDetailsOpen] = useState(false)
  const entries = useMemo(
    () => botChat([], coalesceSessionEvents([...(history.data?.events ?? []), ...live.data]), group, []).entries,
    [history.data?.events, live.data, group],
  )
  // Who is taking a turn comes with the group in the bot list, polled briskly
  // while the room is open so "is working" rows keep up with the round.
  useQuery({ ...botsQuery, refetchInterval: 2_000 })
  const members = useMemo(() => bots.filter((bot) => group.members?.includes(bot.id)), [bots, group.members])
  const working = members.filter((bot) => group.working?.includes(bot.id))
  const reducedMotion = useReducedMotion()

  useEffect(() => {
    void markThreadSeen(group.id).finally(() => invalidateSessionLists(queryClient))
  }, [group.id, queryClient])
  useTitlebarSlot(useMemo(() => <BotPill avatars={botAvatars(group, bots)} name={group.name} />, [group, bots]))
  useTitlebarActions(
    useMemo(
      () => (
        <SidePanelControl
          open={detailsOpen}
          mode="overview"
          modes={['overview']}
          overviewLabel="Details"
          onToggle={() => setDetailsOpen((open) => !open)}
        />
      ),
      [detailsOpen],
    ),
  )
  useWindowEvent('keydown', (e) => {
    if (e.defaultPrevented || !e.metaKey || e.shiftKey || e.altKey || e.ctrlKey || modalDialogOpen()) return
    if (e.key.toLowerCase() !== 'o') return
    e.preventDefault()
    setDetailsOpen((open) => !open)
  })
  const send = async (text: string, options: SendMessageOptions = {}) => {
    try {
      const uploaded = await Promise.all((options.files ?? []).map((file) => uploadSessionAttachment(group.id, file)))
      await sendGroupMessage(group.id, text, preparedSendMessage(options, uploaded).attachmentIds)
    } catch (error) {
      toast(`Couldn't send: ${(error as Error).message}`, 'danger')
      throw error
    }
    pinToBottom()
  }

  return (
    <FileDropScope className="relative flex h-full">
      <div className="flex min-w-0 flex-1 flex-col">
        <div ref={attachScroll} onScroll={onScroll} className="scrollbar-quiet min-h-0 flex-1 overflow-y-auto">
          <div className={`${THREAD_COLUMN_CLASS} py-6`}>
            {history.isError && !history.data ? (
              <EmptyState title="Couldn't load this chat" />
            ) : history.data && !entries.length ? (
              <EmptyState title="Say something to the group" />
            ) : (
              <ChatLog
                entries={entries}
                bots={bots}
                mentionBots={members}
                named
                working={working.map((bot) => ({ bot }))}
                scrollRef={scrollRef}
                paging={history.paging}
              />
            )}
          </div>
        </div>
        <div className={`${THREAD_COLUMN_CLASS} w-full pb-4`}>
          <ComposerCard
            streaming={false}
            placeholder={`Message ${group.name}`}
            showOptions={false}
            clearTiming="immediate"
            draftStorageKey={`jaz.groupDraft.${group.id}`}
            draftStorage="local"
            attachmentSessionId={group.id}
            onSend={send}
            onUploadAttachment={(file) => uploadSessionAttachment(group.id, file)}
          />
        </div>
      </div>
      <motion.div
        initial={false}
        animate={{ width: detailsOpen ? OVERVIEW_PANEL_WIDTH : 0 }}
        transition={reducedMotion ? { duration: 0 } : { type: 'spring', ...panelSpring }}
        inert={!detailsOpen}
        className="h-full shrink-0 overflow-hidden max-sm:absolute max-sm:inset-y-0 max-sm:right-0 max-sm:z-shell"
      >
        <GroupDetails group={group} bots={bots} />
      </motion.div>
    </FileDropScope>
  )
}
