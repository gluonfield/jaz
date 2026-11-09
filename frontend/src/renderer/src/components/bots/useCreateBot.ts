import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useToast } from '@/components/ui/toast'
import { createBot, createGroup } from '@/lib/api/bots'
import type { Bot } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

// A bot exists as soon as it is created; it opens with its details showing.
export function useCreateBot() {
  return useCreate(createBot, 'a bot')
}

export function useCreateGroup() {
  return useCreate(createGroup, 'the group')
}

// A new bot or group joins the list at once and opens.
function useCreate<T>(create: (input: T) => Promise<Bot>, what: string) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const toast = useToast()
  return useMutation({
    mutationFn: create,
    onSuccess: (bot) => {
      queryClient.setQueryData<Bot[]>(keys.bots, (bots = []) => [bot, ...bots])
      void queryClient.invalidateQueries({ queryKey: keys.bots })
      void navigate({ to: '/bots/$botId', params: { botId: bot.id }, state: { newBot: bot.kind === 'bot' } })
    },
    onError: (error) => toast(`Couldn't create ${what}: ${error.message}`, 'danger'),
  })
}
