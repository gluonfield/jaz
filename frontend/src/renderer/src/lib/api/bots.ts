import { queryOptions } from '@tanstack/react-query'
import { keys } from '@/lib/query/keys'
import { del, get, patch, post, put } from './client'
import type { Bot, BotAvatar } from './types'

// Polling belongs to the list on screen; other readers take the shared cache.
export const botsQuery = queryOptions({
  queryKey: keys.bots,
  queryFn: async () => (await get<{ bots: Bot[] | null }>('/v1/bots')).bots ?? [],
  staleTime: 10_000,
})

export function createBot(input: { name: string; avatar: BotAvatar }): Promise<Bot> {
  return post<Bot>('/v1/bots', input)
}

export function createGroup(input: { name: string; members: string[] }): Promise<Bot> {
  return post<Bot>('/v1/bots/groups', input)
}

export type BotPatch = Partial<Pick<Bot, 'name' | 'avatar' | 'members' | 'agent' | 'model' | 'reasoning_effort'>>

export function updateBot(id: string, input: BotPatch): Promise<Bot> {
  return patch<Bot>(`/v1/bots/${id}`, input)
}

// Pins exactly ids, in that order, and unpins every other bot and group.
export function pinBots(ids: string[]): Promise<void> {
  return put<void>('/v1/bots/pins', { ids })
}

export function deleteBot(id: string): Promise<void> {
  return del<void>(`/v1/bots/${id}`)
}

export function sendGroupMessage(id: string, text: string, attachmentIds: string[]): Promise<void> {
  return post<void>(`/v1/bots/${id}/messages`, { text, attachment_ids: attachmentIds })
}
