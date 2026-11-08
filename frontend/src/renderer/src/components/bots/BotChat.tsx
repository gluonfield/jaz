import { useQuery } from '@tanstack/react-query'
import { memo, useEffect, useMemo, useState } from 'react'
import type { ThreadChatView } from '@/components/session/ThreadView'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot, MCPEntrypoint } from '@/lib/api/types'
import { botChat, botWorkActivities, botWorkDescription, type ChatEntry } from '@/lib/bots'
import { deriveBotProgress } from '@/lib/botProgress'
import { BotStarters } from './BotStarters'
import { ChatLog } from './ChatLog'

const NO_ENTRYPOINTS: MCPEntrypoint[] = []
const TYPING_MS = 900

// A bot's thread read as its chat, rebuilt only when the thread changes rather
// than on every render of the thread view around it. The chat opens with the
// bot's hello, as old as the bot; a bot you just created types it first.
export const BotChat = memo(function BotChat({ bot, bots, newBot, empty, messages, events, taskEvents, scrollRef, paging, working, threads, send, showDetails, revealSeq, activity, acpPlan, acpToolCalls, lastEventAt, lastToolAt }: ThreadChatView & { bot: Bot; bots: Bot[]; newBot: boolean }) {
  const entrypoints = useQuery(mcpEntrypointsQuery).data ?? NO_ENTRYPOINTS
  const chat = useMemo(() => botChat(messages, [...events, ...(taskEvents ?? [])], bot, entrypoints), [messages, events, taskEvents, bot, entrypoints])
  const [helloAt, setHelloAt] = useState(newBot ? '' : bot.created_at ?? bot.updated_at)
  useEffect(() => {
    if (helloAt) return
    const timer = setTimeout(() => setHelloAt(new Date().toISOString()), TYPING_MS)
    return () => clearTimeout(timer)
  }, [helloAt])
  const entries = useMemo(
    () => (helloAt && !paging.hasEarlier ? [hello(bot, helloAt), ...chat.entries] : chat.entries),
    [helloAt, paging.hasEarlier, bot, chat.entries],
  )
  const activities = botWorkActivities(chat.tasks, threads)
  const [now, setNow] = useState(Date.now)
  const active = working || activities.length > 0
  useEffect(() => {
    if (!active) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [active])
  const progress = (working || activity?.phase === 'error') && !chat.waiting
    ? deriveBotProgress({ running: true, activity, lastEventAt, lastToolAt, plan: acpPlan, toolCalls: acpToolCalls, now })
    : undefined
  const parentDoing = !helloAt
    ? 'typing'
    : (working || activity?.phase === 'error') && !chat.waiting
      ? progress ?? chat.work.doing ?? 'working'
      : undefined
  const doing = botWorkDescription(parentDoing, chat.tasks, threads)
  const visibleProgress = (progress?.kind === 'processing' || activity?.phase === 'idle') && activities.length ? undefined : progress
  const busy = doing ? [{ bot, doing, progress: visibleProgress, activities }] : []
  return (
    <>
      <ChatLog entries={entries} bots={bots} named={false} working={busy} onOpenRoutines={showDetails} onSendPrompt={send} scrollRef={scrollRef} paging={paging} revealSeq={revealSeq} />
      {helloAt && empty ? <BotStarters arriving={newBot} onPick={send} /> : null}
    </>
  )
})

function hello(bot: Bot, at: string): ChatEntry {
  return { kind: 'bot', key: 'hello', at, botId: bot.id, name: bot.name, text: `Hi, I'm ${bot.name}. Tell me what you need, or pick something to start with.` }
}
