import { useQuery } from '@tanstack/react-query'
import { memo, useEffect, useMemo, useState } from 'react'
import type { ThreadChatView } from '@/components/session/ThreadView'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot, MCPEntrypoint } from '@/lib/api/types'
import { botChat, subtasksDoing, type ChatEntry } from '@/lib/bots'
import { BotStarters } from './BotStarters'
import { ChatLog } from './ChatLog'

const NO_ENTRYPOINTS: MCPEntrypoint[] = []
const TYPING_MS = 900

// A bot's thread read as its chat, rebuilt only when the thread changes rather
// than on every render of the thread view around it. The chat opens with the
// bot's hello, as old as the bot; a bot you just created types it first.
export const BotChat = memo(function BotChat({ bot, bots, newBot, messages, events, scrollRef, paging, working, threads, send, showDetails }: ThreadChatView & { bot: Bot; bots: Bot[]; newBot: boolean }) {
  const entrypoints = useQuery(mcpEntrypointsQuery).data ?? NO_ENTRYPOINTS
  const chat = useMemo(() => botChat(messages, events, bot, entrypoints), [messages, events, bot, entrypoints])
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
  const doing = !helloAt ? 'typing' : working && !chat.waiting ? chat.work.doing ?? 'working' : subtasksDoing(threads)
  const busy = doing ? [{ bot, doing }] : []
  return (
    <>
      <ChatLog entries={entries} bots={bots} named={false} working={busy} onOpenRoutines={showDetails} scrollRef={scrollRef} paging={paging} />
      {helloAt && !chat.entries.some((entry) => entry.kind === 'user') ? <BotStarters arriving={newBot} onPick={send} /> : null}
    </>
  )
})

function hello(bot: Bot, at: string): ChatEntry {
  return { kind: 'bot', key: 'hello', at, botId: bot.id, name: bot.name, text: `Hi, I'm ${bot.name}. Tell me what you need, or pick something to start with.` }
}
