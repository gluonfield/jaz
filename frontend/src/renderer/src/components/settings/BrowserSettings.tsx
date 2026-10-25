import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BrowserProfileImport } from '@/components/browser/BrowserProfileImport'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { browserSettingsQuery, updateBrowserSettings } from '@/lib/api/settings'
import { keys } from '@/lib/query/keys'

export function BrowserSettings() {
  const status = useQuery(browserSettingsQuery)
  const queryClient = useQueryClient()
  const toast = useToast()
  const toggle = useMutation({
    mutationFn: updateBrowserSettings,
    onSuccess: (next) => queryClient.setQueryData(keys.browserSettings, next),
    onError: (error: Error) => toast(`Couldn't update browser settings: ${error.message}`, 'danger'),
  })

  if (status.isPending) {
    return <Skeleton className="my-4 h-28" />
  }
  if (status.isError) {
    return <EmptyState title="Couldn't load browser settings">{status.error.message}</EmptyState>
  }
  return (
    <section className="py-4">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-ink">Browser</h1>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">
            Agent browsing opens this conversation’s side browser.
          </p>
        </div>
        <Switch
          checked={status.data.enabled}
          disabled={toggle.isPending}
          onChange={(enabled) => toggle.mutate(enabled)}
          aria-label="Enable browser tools"
        />
      </header>
      <div className="mt-4"><BrowserProfileImport /></div>
    </section>
  )
}
