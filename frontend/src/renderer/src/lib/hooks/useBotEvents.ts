import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { openEventStream } from '@/lib/api/eventStream'
import type { Bot } from '@/lib/api/types'
import type { BotListEntry } from '@/lib/api/bots'
import { useConnection } from '@/lib/connection'
import { keys } from '@/lib/query/keys'

// Snapshots establish the baseline on every connection. Only live updates
// advance the completion marker, including runs shorter than a render frame.
function openBotEvents(client: QueryClient): () => void {
  const completions = new Map<string, string | undefined>()
  return openEventStream('/v1/bots/events', {
    snapshot: (event) => {
      const { bots } = JSON.parse(event.data) as { bots: Bot[] | null }
      completions.clear()
      for (const bot of bots ?? []) {
        completions.set(bot.id, bot.completed_at)
      }
      void client.cancelQueries({ queryKey: keys.bots, exact: true })
      client.setQueryData(keys.bots, bots ?? [])
      void client.invalidateQueries({ queryKey: keys.bots, predicate: (query) => query.queryKey.length > 1 })
    },
    changed: (event) => {
      const bot = JSON.parse(event.data) as Bot
      const completed = completions.has(bot.id) && bot.completed_at && bot.completed_at !== completions.get(bot.id)
      completions.set(bot.id, bot.completed_at)
      void client.cancelQueries({ queryKey: keys.bots, exact: true })
      client.setQueryData<BotListEntry[]>(keys.bots, (items = []) => {
        const previous = items.find((item) => item.id === bot.id)
        const celebrate = completed ? bot.completed_at : previous?.celebrate
        const next = { ...bot, celebrate }
        return previous ? items.map((item) => item.id === bot.id ? next : item) : [...items, next]
      })
      void client.invalidateQueries({ queryKey: keys.botActivity(bot.id) })
    },
    removed: (event) => {
      const id = JSON.parse(event.data) as string
      completions.delete(id)
      client.setQueryData<Bot[]>(keys.bots, (items = []) => items.filter((item) => item.id !== id))
    },
  })
}

export function useBotEvents() {
  const client = useQueryClient()
  const { url, status } = useConnection()
  const connected = status === 'connected' || status === 'reconnecting'
  useEffect(() => {
    if (connected) {
      return openBotEvents(client)
    }
  }, [client, url, connected])
}
