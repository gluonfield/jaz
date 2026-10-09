import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useLocation } from '@tanstack/react-router'
import { useEffect } from 'react'
import { BotPill } from '@/components/bots/BotAvatar'
import { BotDetails } from '@/components/bots/BotDetails'
import { BotChat } from '@/components/bots/BotChat'
import { GroupChat } from '@/components/bots/GroupChat'
import { ThreadView } from '@/components/session/ThreadView'
import { EmptyState } from '@/components/ui/EmptyState'
import { botsQuery } from '@/lib/api/bots'
import { rememberBot } from '@/lib/bots'

declare module '@tanstack/history' {
  interface HistoryState {
    newBot?: boolean
  }
}

export const Route = createFileRoute('/bots/$botId')({
  component: BotRoute,
})

function BotRoute() {
  const { botId } = Route.useParams()
  const newBot = useLocation({ select: (location) => Boolean(location.state.newBot) })
  const bots = useQuery(botsQuery)
  const bot = bots.data?.find((item) => item.id === botId)
  useEffect(() => rememberBot(botId), [botId])
  if (!bot) return bots.isPending ? null : <EmptyState title="This bot is gone" />
  if (bot.kind === 'group') return <GroupChat key={bot.id} group={bot} bots={bots.data ?? []} />
  // A bot is a thread with a face: the thread view shown as a chat, its
  // identity in the titlebar, and its details where Overview would be.
  return (
    <ThreadView
      key={bot.id}
      sessionId={bot.id}
      header={<BotPill avatars={[bot.avatar]} name={bot.name} />}
      details={(view) => <BotDetails bot={bot} focusName={newBot} {...view} />}
      openDetails={newBot}
      placeholder={`Message ${bot.name}`}
      chat={(view) => <BotChat bot={bot} bots={bots.data ?? []} {...view} />}
    />
  )
}
