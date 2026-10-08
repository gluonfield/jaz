import { useQuery } from '@tanstack/react-query'
import { memo, useMemo } from 'react'
import type { ThreadChatView } from '@/components/session/ThreadView'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { Bot, MCPEntrypoint } from '@/lib/api/types'
import { botChat, subtasksDoing } from '@/lib/bots'
import { BotWelcome } from './BotWelcome'
import { ChatLog } from './ChatLog'

const NO_ENTRYPOINTS: MCPEntrypoint[] = []

// A bot's thread read as its chat, rebuilt only when the thread changes rather
// than on every render of the thread view around it.
export const BotChat = memo(function BotChat({ bot, bots, messages, events, scrollRef, paging, working, threads, send, showDetails }: ThreadChatView & { bot: Bot; bots: Bot[] }) {
  const entrypoints = useQuery(mcpEntrypointsQuery).data ?? NO_ENTRYPOINTS
  const chat = useMemo(() => botChat(messages, events, bot, entrypoints), [messages, events, bot, entrypoints])
  if (!chat.entries.length && !working) return <BotWelcome onPick={send} />
  const subtasks = subtasksDoing(threads)
  const busy = working && !chat.waiting ? [{ bot, ...chat.work }] : subtasks ? [{ bot, doing: subtasks }] : []
  return <ChatLog entries={chat.entries} bots={bots} named={false} working={busy} onOpenRoutines={showDetails} scrollRef={scrollRef} paging={paging} />
})
