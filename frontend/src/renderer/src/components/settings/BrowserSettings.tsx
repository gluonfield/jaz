import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BrowserProfileImport } from '@/components/browser/BrowserProfileImport'
import { EmptyState } from '@/components/ui/EmptyState'
import { Select } from '@/components/ui/Select'
import { Skeleton } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { browserSettingsQuery, updateBrowserSettings } from '@/lib/api/settings'
import { keys } from '@/lib/query/keys'
import { clientRuntime } from '@/lib/clientRuntime'
import { useResponseLinksExternal } from '@/lib/responseLinks'
import { SettingsCard, SettingsRow } from './SettingsCard'

export function BrowserSettings() {
  const [external, setExternal] = useResponseLinksExternal()
  return (
    <section className="py-4">
      <BrowserTools />
      <SettingsCard className="mt-4">
        <SettingsRow title="Links in replies" description="Choose where web links in assistant and bot replies open on this device.">
          <Select
            aria-label="Open links in replies"
            value={external ? 'external' : 'embedded'}
            options={[
              { value: 'embedded', label: 'Built-in browser' },
              { value: 'external', label: clientRuntime.kind === 'web' ? 'New browser tab' : 'Default browser' },
            ]}
            onChange={(value) => setExternal(value === 'external')}
          />
        </SettingsRow>
      </SettingsCard>
      <div className="mt-4"><BrowserProfileImport /></div>
    </section>
  )
}

function BrowserTools() {
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
  )
}
