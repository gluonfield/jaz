import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { MenuAction } from '@/components/ui/ActionMenu'
import { useToast } from '@/components/ui/toast'
import { deleteApp, hasBundle, type JazApp } from '@/lib/api/apps'
import { useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

// Deleting a board keeps what was on it.
const warning = (app: JazApp) => hasBundle(app)
  ? `Delete "${app.title}" and all its stored data?`
  : `Delete board "${app.title}"? Its apps are kept. A loop on no other board stops publishing until you add it to one.`

// The Delete entry of an app's menu: none for someone who may not delete it.
export function useDeleteApp(onDeleted?: () => void): (app: JazApp) => MenuAction[] {
  const queryClient = useQueryClient()
  const toast = useToast()
  const isViewer = useIsCloudViewer()
  const remove = useMutation({
    mutationFn: (app: JazApp) => deleteApp(app.id),
    onSuccess: (_, app) => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast(hasBundle(app) ? 'Deleted app' : 'Deleted board')
      onDeleted?.()
    },
    onError: (err) => toast(err.message, 'danger'),
  })
  return (app) => isViewer || app.can_edit === false ? [] : [{
    label: 'Delete',
    onSelect: () => {
      if (window.confirm(warning(app))) remove.mutate(app)
    },
  }]
}
