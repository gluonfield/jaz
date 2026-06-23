import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCcw } from 'lucide-react'
import { SettingsCard } from './SettingsCard'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Select } from '@/components/ui/Select'
import { Skeleton } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { browserSettingsQuery, agentSettingsQuery, updateBrowserSettings } from '@/lib/api/settings'
import type { BrowserStatus } from '@/lib/api/types'
import { enabledACPAgents } from '@/lib/agentRuntimes'
import { keys } from '@/lib/query/keys'

function formatTime(value?: string): string {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'never'
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function BrowserSettings() {
  const status = useQuery(browserSettingsQuery)
  const agentSettings = useQuery(agentSettingsQuery)
  const queryClient = useQueryClient()
  const toast = useToast()

  const setStatus = (next: BrowserStatus) => queryClient.setQueryData(keys.browserSettings, next)

  const toggle = useMutation({
    mutationFn: (enabled: boolean) => updateBrowserSettings({ enabled }),
    onSuccess: setStatus,
    onError: (error: Error) => toast(`Couldn't update browser settings: ${error.message}`, 'danger'),
  })

  const setAgent = useMutation({
    mutationFn: (agent: string) => updateBrowserSettings({ agent }),
    onSuccess: setStatus,
    onError: (error: Error) => toast(`Couldn't update browser agent: ${error.message}`, 'danger'),
  })

  if (status.isPending) {
    return (
      <section className="py-5">
        <Skeleton className="mb-4 h-7 w-40" />
        <Skeleton className="mb-4 h-28" />
        <Skeleton className="h-40" />
      </section>
    )
  }

  if (status.isError) {
    return (
      <EmptyState title="Couldn't load browser settings">
        <p>{status.error.message}</p>
      </EmptyState>
    )
  }

  const browser = status.data
  const extension = browser.extension ?? { connected: false }
  const agents = enabledACPAgents(agentSettings.data).filter((agent) => agent !== 'jaz')
  const selectedAgent = browser.agent ?? ''
  const staleAgent = selectedAgent && !agents.includes(selectedAgent)
  const agentOptions = [
    { value: '', label: 'Not selected', description: 'Browser tools will stay unavailable' },
    ...(staleAgent
      ? [
          {
            value: selectedAgent,
            label: `${agentLabel(selectedAgent)} (disabled)`,
            description: 'Enable this ACP agent or choose another one',
          },
        ]
      : []),
    ...agents.map((agent) => ({
      value: agent,
      label: agentLabel(agent),
      description: 'Runs delegated browser tasks',
    })),
  ]
  const agentValid = !selectedAgent || agents.includes(selectedAgent) || agentSettings.isPending
  const connected = Boolean(extension.connected)

  return (
    <section className="py-5">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-ink">Browser</h1>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">
            Delegated browser workers use the selected coding agent and prefer the connected Chrome extension.
          </p>
        </div>
        <div className="flex h-8 shrink-0 items-center gap-2">
          <span className="text-[12px] text-ink-2">{browser.enabled ? 'Enabled' : 'Disabled'}</span>
          <Switch
            checked={browser.enabled}
            disabled={toggle.isPending}
            onChange={(enabled) => toggle.mutate(enabled)}
            aria-label="Enable browser tools"
          />
        </div>
      </header>

      <SettingsCard className="mt-4 px-4 py-3">
        <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_260px] md:items-center">
          <div className="min-w-0">
            <p className="text-[13px] font-medium text-ink">Browser worker agent</p>
            <p className="mt-0.5 text-[12px] text-ink-2">
              Raw page state stays inside this child ACP session.
            </p>
            {!agentValid ? (
              <p className="mt-1 text-[12px] text-danger">
                {agentLabel(selectedAgent)} is no longer enabled.
              </p>
            ) : null}
          </div>
          <Select
            value={selectedAgent}
            options={agentOptions}
            disabled={!browser.enabled || setAgent.isPending || agentSettings.isPending}
            onChange={(agent) => setAgent.mutate(agent)}
            aria-label="Browser worker agent"
          />
        </div>
      </SettingsCard>

      <SettingsCard className="mt-4 overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
          <div>
            <p className="text-[13px] font-medium text-ink">Chrome extension</p>
            <p className="mt-0.5 text-[12px] text-ink-2">
              {connected ? 'Signed-in Chrome bridge is active.' : 'No extension bridge is connected.'}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <span
              className={`rounded-full px-2 py-0.5 text-[12px] font-medium ${
                connected ? 'bg-primary-soft text-ink' : 'bg-surface-2 text-ink-2'
              }`}
            >
              {connected ? 'Connected' : 'Disconnected'}
            </span>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void status.refetch()}
              disabled={status.isFetching}
              aria-label="Refresh browser extension status"
            >
              <RefreshCcw size={14} />
              Refresh
            </Button>
          </div>
        </div>

        <dl className="grid grid-cols-1 gap-x-4 gap-y-3 px-4 py-3 text-[12px] md:grid-cols-[140px_minmax(0,1fr)]">
          <dt className="text-ink-3">Bridge URL</dt>
          <dd className="min-w-0 break-all font-mono text-ink">
            {extension.bridge_url || 'Not reported'}
          </dd>
          <dt className="text-ink-3">Extension ID</dt>
          <dd className="min-w-0 break-all font-mono text-ink">
            {extension.extension_id || 'Not connected'}
          </dd>
          <dt className="text-ink-3">Last connected</dt>
          <dd className="text-ink">{formatTime(extension.last_connected_at)}</dd>
          <dt className="text-ink-3">Actions</dt>
          <dd className="min-w-0 text-ink-2">
            {extension.actions?.length ? extension.actions.join(', ') : 'Not reported'}
          </dd>
        </dl>
      </SettingsCard>
    </section>
  )
}
