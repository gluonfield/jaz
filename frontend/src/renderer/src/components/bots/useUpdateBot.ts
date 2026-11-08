import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/components/ui/toast'
import { type BotPatch, updateBot } from '@/lib/api/bots'
import type { Bot } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

// Name, face, and members apply to the cached list at once so every surface
// showing the bot follows the edit; the server copy settles it.
export function useUpdateBot(id: string) {
  const queryClient = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: (input: BotPatch) => updateBot(id, input),
    onMutate: (input) => {
      const optimistic = Object.fromEntries(Object.entries(input).filter(([key]) => ['name', 'avatar', 'members'].includes(key)))
      queryClient.setQueryData<Bot[]>(keys.bots, (bots) =>
        bots?.map((bot) => (bot.id === id ? { ...bot, ...optimistic } : bot)),
      )
    },
    onSuccess: (updated) => queryClient.setQueryData<Bot[]>(keys.bots, (bots) =>
      bots?.map((bot) => bot.id === id ? { ...bot, ...updated } : bot),
    ),
    onError: (error) => toast(`Couldn't save: ${error.message}`, 'danger'),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.bots }),
  })
}
