import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Clock3, Loader2, Mail, Plug, Unplug } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  connectionPluginsQuery,
  disconnectConnectionAccount,
  startConnectionPlugin,
} from '@/lib/api/connections'
import { clientRuntime } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'
import type {
  IntegrationCapability,
  IntegrationConnectionAccount,
  IntegrationPlugin,
} from '@/lib/api/types'

const CAPABILITY_LABELS: Record<IntegrationCapability, string> = {
  sync: 'Sync',
  act: 'Actions',
  materialize: 'Memory',
  mcp: 'MCP',
  browser: 'Browser',
}

export function ConnectionsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [pollUntil, setPollUntil] = useState(0)
  const plugins = useQuery({
    ...connectionPluginsQuery,
    refetchInterval: () => (Date.now() < pollUntil ? 2000 : false),
  })
  const connect = useMutation({
    mutationFn: (id: string) => startConnectionPlugin(id),
    onSuccess: (result) => {
      setPollUntil(Date.now() + 90_000)
      openAuthURL(result.auth_url)
      toast('Finish Gmail sign-in in your browser')
    },
    onError: (error: Error) => {
      toast(`Couldn't start Gmail sign-in: ${error.message}`, 'danger')
    },
  })
  const disconnect = useMutation({
    mutationFn: disconnectConnectionAccount,
    onSuccess: () => toast('Disconnected account'),
    onError: (error: Error) => toast(`Couldn't disconnect account: ${error.message}`, 'danger'),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.connectionPlugins }),
  })
  const sortedPlugins = useMemo(
    () => [...(plugins.data ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [plugins.data],
  )

  useEffect(() => {
    if (pollUntil === 0) return
    const refresh = () => {
      if (document.visibilityState === 'hidden') return
      void queryClient.invalidateQueries({ queryKey: keys.connectionPlugins })
    }
    const timeout = window.setTimeout(() => setPollUntil(0), Math.max(0, pollUntil - Date.now()))
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    return () => {
      window.clearTimeout(timeout)
      window.removeEventListener('focus', refresh)
      document.removeEventListener('visibilitychange', refresh)
    }
  }, [pollUntil, queryClient])

  return (
    <section className="py-5">
      <div>
        <p className="text-sm font-medium text-ink">Connections</p>
        <p className="mt-0.5 text-[13px] text-ink-2">
          First-party app connections for sync, actions, and memory materialization.
        </p>
      </div>

      <div className="mt-4">
        {plugins.isPending ? (
          <SkeletonRows count={3} />
        ) : plugins.isError ? (
          <p className="py-2 text-[13px] text-danger">{plugins.error.message}</p>
        ) : sortedPlugins.length === 0 ? (
          <p className="rounded-card bg-surface px-3 py-3 text-[13px] text-ink-3">
            No first-party connections are available yet.
          </p>
        ) : (
          <div className="flex flex-col gap-px">
            {sortedPlugins.map((plugin) => (
              <ConnectionPluginRow
                key={plugin.id}
                plugin={plugin}
                connecting={connect.isPending && connect.variables === plugin.id}
                disconnectingAccountID={disconnect.isPending ? disconnect.variables : undefined}
                onConnect={() => connect.mutate(plugin.id)}
                onDisconnect={(account) => {
                  const label = accountLabel(account) || account.id
                  if (window.confirm(`Disconnect ${label}?`)) disconnect.mutate(account.id)
                }}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  )
}

function openAuthURL(url: string): void {
  if (clientRuntime.openExternalURL) {
    clientRuntime.openExternalURL(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

function ConnectionPluginRow({
  plugin,
  connecting,
  disconnectingAccountID,
  onConnect,
  onDisconnect,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  disconnectingAccountID?: string
  onConnect: () => void
  onDisconnect: (account: IntegrationConnectionAccount) => void
}) {
  const toolCount = plugin.tools?.length ?? 0
  const sourceLanes = plugin.source_lanes ?? []
  const available = plugin.implementation.status === 'available'
  const connected = plugin.connection?.status === 'connected'
  const accounts = plugin.connection?.accounts ?? []
  const actionLabel = connecting ? 'Connecting' : connected && plugin.multi_account ? 'Add account' : connected ? 'Reconnect' : 'Connect'
  const actionIcon = connecting ? (
    <Loader2 size={13} className="animate-spin" />
  ) : connected && plugin.multi_account ? (
    <Plug size={13} />
  ) : connected ? (
    <CheckCircle2 size={13} />
  ) : (
    <Plug size={13} />
  )

  return (
    <div className="flex items-start gap-3 rounded-card px-3 py-3 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface max-sm:flex-col">
      <div className="flex min-w-0 flex-1 gap-3">
        <PluginIcon plugin={plugin} />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <span className="truncate font-medium text-ink" title={plugin.name}>
              {plugin.name}
            </span>
            {plugin.multi_account ? <Pill>Multiple accounts</Pill> : null}
            {connected ? <ConnectedPill /> : null}
          </div>
          {plugin.description ? <p className="mt-1 text-[12px] leading-5 text-ink-3">{plugin.description}</p> : null}
          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            {plugin.capabilities.map((capability) => (
              <Pill key={capability}>{CAPABILITY_LABELS[capability]}</Pill>
            ))}
          </div>
          <p className="mt-2 text-[12px] text-ink-3">
            {toolCount > 0 ? `${toolCount} tool specs` : 'No tool specs yet'}
            {sourceLanes.length > 0 ? ` - ${sourceLanes.join(', ')}` : ''}
            {plugin.remote_mcp ? ` - Official MCP ${statusLabel(plugin.remote_mcp.status)}` : ''}
          </p>
          {accounts.length > 0 ? (
            <div className="mt-2 flex flex-col gap-1.5">
              {accounts.map((account) => (
                <ConnectedAccountRow
                  key={account.id}
                  account={account}
                  disconnecting={disconnectingAccountID === account.id}
                  onDisconnect={() => onDisconnect(account)}
                />
              ))}
            </div>
          ) : null}
        </div>
      </div>

      {available ? (
        <Button variant="secondary" size="sm" disabled={connecting} onClick={onConnect} className="max-sm:self-start">
          {actionIcon}
          {actionLabel}
        </Button>
      ) : (
        <span
          className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full bg-surface-2 px-2.5 text-[12px] font-medium text-ink-3 max-sm:self-start"
          title="The first-party account connection flow is not implemented yet."
        >
          <Clock3 size={13} />
          {statusLabel(plugin.implementation.status)}
        </span>
      )}
    </div>
  )
}

function ConnectedAccountRow({
  account,
  disconnecting,
  onDisconnect,
}: {
  account: IntegrationConnectionAccount
  disconnecting: boolean
  onDisconnect: () => void
}) {
  const label = accountLabel(account) || account.id
  const sublabel = accountSubLabel(account, label)
  return (
    <div className="flex min-w-0 items-center justify-between gap-2 rounded-control bg-surface px-2.5 py-2 max-sm:flex-col max-sm:items-start">
      <div className="min-w-0">
        <p className="truncate text-[12px] font-medium text-ink" title={label}>
          {label}
        </p>
        {sublabel ? (
          <p className="truncate text-[11px] text-ink-3" title={sublabel}>
            {sublabel}
          </p>
        ) : null}
      </div>
      <Button
        variant="danger"
        size="sm"
        disabled={disconnecting}
        onClick={onDisconnect}
        className="min-h-10 whitespace-nowrap max-sm:self-start"
      >
        {disconnecting ? <Loader2 size={13} className="animate-spin" /> : <Unplug size={13} />}
        {disconnecting ? 'Disconnecting' : 'Disconnect'}
      </Button>
    </div>
  )
}

function accountLabel(account: IntegrationConnectionAccount): string {
  if (account.account_name) return account.account_name
  if (account.account_id) return account.account_id
  if (account.alias && account.alias !== 'default') return account.alias
  return ''
}

function accountSubLabel(account: IntegrationConnectionAccount, label: string): string {
  if (account.alias && account.alias !== 'default' && account.alias !== label) return account.alias
  return ''
}

function ConnectedPill() {
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-ok/10 px-1.5 py-[3px] text-[11px] leading-none text-ok">
      <CheckCircle2 size={11} />
      Connected
    </span>
  )
}

function PluginIcon({ plugin }: { plugin: IntegrationPlugin }) {
  if (plugin.icon.kind === 'url') {
    return (
      <img
        src={plugin.icon.value}
        alt=""
        className="size-9 shrink-0 rounded-control bg-surface-2 object-cover"
      />
    )
  }

  if (plugin.icon.kind === 'asset' && plugin.icon.value === 'gmail') {
    return (
      <span className="grid size-9 shrink-0 place-items-center rounded-control bg-surface-2 text-[#d93025]">
        <Mail size={18} />
      </span>
    )
  }

  return (
    <span
      className="grid size-9 shrink-0 place-items-center rounded-control bg-surface-2 text-[12px] font-medium text-ink"
      style={plugin.icon.background ? { background: plugin.icon.background } : undefined}
    >
      {plugin.icon.value || plugin.name.slice(0, 2).toUpperCase()}
    </span>
  )
}

function Pill({ children }: { children: string }) {
  return (
    <span className="rounded-full bg-surface-2 px-1.5 py-[3px] text-[11px] leading-none text-ink-2">
      {children}
    </span>
  )
}

function statusLabel(status: string): string {
  return status
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
