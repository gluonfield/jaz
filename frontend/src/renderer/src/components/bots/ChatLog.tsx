import { memo, useMemo, type RefObject } from 'react'
import { MCPAppFrame } from '@/components/apps/MCPAppFrame'
import { MessageActions } from '@/components/session/MessageActions'
import { MessageAttachments } from '@/components/session/MessageAttachments'
import { PermissionCard } from '@/components/session/TranscriptPermissions'
import { UserMessageMarkdown } from '@/components/session/MessageMarkdown'
import { SystemEventRow } from '@/components/session/SystemEventRow'
import { useHistoryWindow } from '@/components/session/useHistoryWindow'
import type { HistoryPaging } from '@/lib/hooks/useSessionHistory'
import type { Bot, BotAvatar as Avatar } from '@/lib/api/types'
import { botInk, botTarget, type BotWork, type ChatEntry } from '@/lib/bots'
import { messageTime } from '@/lib/format/time'
import { BotAvatar } from './BotAvatar'

const GONE: Avatar = { shape: 'circle', color: 'gray' }
const QUIET_GAP_MS = 30 * 60_000
const INITIAL_VISIBLE_ENTRIES = 60
const VISIBLE_ENTRY_BATCH = 60

// A messenger-style log shared by a bot's chat and a group: bubbles, the time
// after a quiet gap, activity rows, and who is working now and on what. `named` labels each speaker with a name and face, which only a
// group needs.
export function ChatLog({
  entries,
  bots,
  mentionBots = bots,
  named,
  working,
  onOpenRoutines,
  scrollRef,
  paging,
}: {
  entries: ChatEntry[]
  bots: Bot[]
  mentionBots?: Bot[]
  named: boolean
  working: ({ bot: Bot } & BotWork)[]
  // Opens the routines a "Created routine" line names.
  onOpenRoutines?: () => void
  scrollRef: RefObject<HTMLDivElement | null>
  paging: HistoryPaging
}) {
  const avatar = (id?: string) => bots.find((bot) => bot.id === id)?.avatar ?? GONE
  const mentions = useMemo(() => {
    const targets = new Map<string, string>()
    for (const bot of mentionBots) {
      targets.set(bot.name, targets.has(bot.name) ? '' : botTarget(bot.id))
    }
    return targets
  }, [mentionBots])
  const { start, historyRef, sentinelRef } = useHistoryWindow({
    scrollRef,
    count: entries.length,
    initial: INITIAL_VISIBLE_ENTRIES,
    batch: VISIBLE_ENTRY_BATCH,
    keyAt: (index) => entries[index]?.key,
    paging,
  })
  return (
    <div ref={historyRef} className="@container relative flex flex-col" aria-busy={paging.loading}>
      {entries.slice(start).map((entry, offset) => {
        const index = start + offset
        const previous = entries[index - 1]
        const next = entries[index + 1]
        const stamped = !previous || Date.parse(entry.at) - Date.parse(previous.at) > QUIET_GAP_MS
        const nextStamped = !next || Date.parse(next.at) - Date.parse(entry.at) > QUIET_GAP_MS
        const sameSpeaker = (other?: ChatEntry) =>
          other?.kind === entry.kind && (entry.kind !== 'bot' || (other.kind === 'bot' && other.botId === entry.botId))
        const opensRun = stamped || !sameSpeaker(previous)
        const closesRun = nextStamped || !sameSpeaker(next)
        return (
          <div key={entry.key} className={opensRun ? 'mt-4 first:mt-0' : 'mt-1'}>
            {stamped ? <p className="pb-3 text-center text-[12px] text-ink-3">{messageTime(entry.at)}</p> : null}
            {entry.kind === 'user' ? (
              <>
                <div className="flex flex-col items-end">
                  <MessageAttachments
                    attachments={entry.attachments ?? []}
                    attachmentSessionId={entry.attachmentSessionId}
                  />
                </div>
                {entry.text ? <ChatBubble text={entry.text} at={entry.at} mentions={mentions} mine /> : null}
              </>
            ) : entry.kind === 'activity' ? (
              <SystemEventRow event={entry.event} onOpen={entry.event.loop_created ? onOpenRoutines : undefined} />
            ) : entry.kind === 'app' ? (
              <MCPAppFrame app={entry.app} call={entry.app} active />
            ) : entry.kind === 'question' ? (
              <PermissionCard event={entry.event} resolution={entry.answer} />
            ) : (
              <div className="flex items-end gap-2.5">
                {named ? (
                  <span className="w-7 shrink-0">{closesRun ? <BotAvatar avatar={avatar(entry.botId)} size={28} /> : null}</span>
                ) : null}
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  {named && opensRun ? (
                    <span
                      className="px-1 text-[12px] font-medium"
                      style={{ color: botInk(avatar(entry.botId).color) }}
                    >
                      {entry.name}
                    </span>
                  ) : null}
                  <ChatBubble text={entry.text} at={entry.at} mentions={mentions} />
                </div>
              </div>
            )}
          </div>
        )
      })}
      {working.map(({ bot, doing = 'working' }) => (
        <div key={bot.id} role="status" className="mt-4 flex items-start gap-2 text-sm text-ink-3 first:mt-0">
          <BotAvatar avatar={bot.avatar} size={22} working />
          <div className="flex min-w-0 flex-col gap-0.5 pt-px">
            <p>
              <span className="live-shimmer">
                {bot.name} is {doing}…
              </span>
            </p>
          </div>
        </div>
      ))}
      <div ref={sentinelRef} className="pointer-events-none absolute inset-x-0 top-0 h-px" aria-hidden />
    </div>
  )
}

// One message, rounded like a messenger's: the person's own on the right in the
// brand tint, a bot's on the left, with copy and time beside it on hover. A
// narrow log drops them so they never squeeze the message.
const ChatBubble = memo(function ChatBubble({ text, at, mentions, mine = false }: {
  text: string
  at: string
  mentions: ReadonlyMap<string, string>
  mine?: boolean
}) {
  return (
    <div className={`group/message flex items-center gap-3 ${mine ? 'flex-row-reverse' : ''}`}>
      <div
        className={`min-w-0 max-w-[84%] rounded-[20px] px-4 py-2.5 text-sm [overflow-wrap:break-word] select-text ${
          mine ? 'bg-bubble-sent' : 'bg-surface'
        }`}
      >
        <UserMessageMarkdown text={text} mentions={mentions} />
      </div>
      <div className="shrink-0 whitespace-nowrap @max-xl:hidden">
        <MessageActions text={text} createdAt={at} />
      </div>
    </div>
  )
})
