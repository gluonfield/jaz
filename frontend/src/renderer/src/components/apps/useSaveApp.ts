import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/components/ui/toast'
import { saveApp, type JazApp } from '@/lib/api/apps'
import { keys } from '@/lib/query/keys'

// Saves an artifact or document to the user's private Apps library in one step.
export function useSaveApp(onSaved?: (app: JazApp) => void) {
  const queryClient = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: saveApp,
    onSuccess: (app) => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast(`Saved "${app.title}" to Apps`)
      onSaved?.(app)
    },
    onError: (err) => toast(err instanceof Error ? err.message : 'Could not save the app', 'danger'),
  })
}
