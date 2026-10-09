import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ComputerPermission } from '@shared/computerControl'
import { Button } from '@/components/ui/Button'
import { Switch } from '@/components/ui/Switch'
import { EmptyState } from '@/components/ui/EmptyState'
import { useToast } from '@/components/ui/toast'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { PERMISSION_NAMES } from '@/components/settings/PermissionGuide'
import { computerSettingsQuery, updateComputerSettings } from '@/lib/api/settings'
import { keys } from '@/lib/query/keys'

export function ComputerSettings() {
  const host = window.jaz?.computer
  const settings = useQuery(computerSettingsQuery)
  const status = useQuery({
    queryKey: keys.computerStatus,
    queryFn: () => host!.status(),
    enabled: Boolean(host),
    refetchInterval: 3000,
  })
  const client = useQueryClient()
  const toast = useToast()
  const toggle = useMutation({
    mutationFn: updateComputerSettings,
    onSuccess: (next) => client.setQueryData(keys.computerSettings, next),
    onError: (error: Error) => toast(error.message, 'danger'),
  })
  const allow = useMutation({
    mutationFn: (permission: ComputerPermission) => host!.allow(permission),
    onError: (error: Error) => toast(error.message, 'danger'),
  })
  if (settings.isError) {
    return <EmptyState title="Couldn't load computer settings">{settings.error.message}</EmptyState>
  }
  const permissions = status.data?.permissions
  return (
    <section className="py-4">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-ink">Computer Use</h1>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">Let agents see and control installed apps on this computer.</p>
        </div>
        <div className="flex min-h-10 items-center">
          <Switch
            className="before:absolute before:-inset-x-1 before:-inset-y-3"
            checked={settings.data?.enabled ?? false}
            disabled={settings.isPending || toggle.isPending || !host}
            onChange={(enabled) => toggle.mutate(enabled)}
            aria-label="Enable computer tools"
          />
        </div>
      </header>
      {!host ? <p className="mt-4 text-sm text-ink-2">Open Jaz desktop to use computer control.</p> : (
        <SettingsCard className="mt-4 px-4 py-3">
          {permissions ? (
            <div className="space-y-1">
              {(Object.keys(PERMISSION_NAMES) as ComputerPermission[]).map((name) => (
                <div key={name} className="flex min-h-8 items-center justify-between gap-4 text-[13px]">
                  <span className="text-ink">{PERMISSION_NAMES[name]}</span>
                  {permissions[name]
                    ? <span className="text-ink-2">Allowed</span>
                    : <Button variant="primary" disabled={allow.isPending} onClick={() => allow.mutate(name)}>Allow</Button>}
                </div>
              ))}
            </div>
          ) : <p className="text-[13px] text-ink-2">{status.error?.message || status.data?.reason || (status.isPending ? 'Checking availability…' : 'Ready')}</p>}
          {status.data?.owner && <p className="mt-3 text-[13px] text-ink-2">An agent is controlling this computer.</p>}
        </SettingsCard>
      )}
    </section>
  )
}
