import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  connectionQRStatus,
  connectionPluginsQuery,
  disconnectConnectionAccount,
  startConnectionPlugin,
} from '@/lib/api/connections'
import { clientRuntime } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'
import type {
  ConnectionQRStart,
  IntegrationConnectionAccount,
  IntegrationPlugin,
} from '@/lib/api/types'
import {
  ConnectionPluginCard,
  ExistingConnectionCard,
  SettingsBlock,
} from './ConnectionCards'
import { ConnectionPluginModal } from './ConnectionPluginModal'
import { ConnectionQRModal } from './ConnectionQRModal'
import { accountAddress } from './connectionFormatting'

type ActiveQR = {
  plugin: IntegrationPlugin
  qr: ConnectionQRStart
}

export function ConnectionsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [pollUntil, setPollUntil] = useState(0)
  const [selectedPluginID, setSelectedPluginID] = useState<string | null>(null)
  const [activeQR, setActiveQR] = useState<ActiveQR | null>(null)
  const plugins = useQuery({
    ...connectionPluginsQuery,
    refetchInterval: () => (Date.now() < pollUntil ? 2000 : false),
  })
  const qrStatus = useQuery({
    queryKey: keys.connectionQR(activeQR?.qr.session_id ?? ''),
    queryFn: () => connectionQRStatus(activeQR?.qr.session_id ?? ''),
    enabled: Boolean(activeQR),
    refetchInterval: (query) => {
      const status = query.state.data?.status ?? activeQR?.qr.status
      return status === 'pending' || status === 'scanned' ? 2000 : false
    },
  })
  const connect = useMutation({
    mutationFn: (id: string) => startConnectionPlugin(id),
    onSuccess: (result) => {
      if (result.type === 'oauth' && result.auth_url) {
        setPollUntil(Date.now() + 90_000)
        openAuthURL(result.auth_url)
        toast('Finish sign-in in your browser')
        return
      }
      if (result.type === 'qr' && result.qr) {
        const plugin = sortedPlugins.find((item) => item.id === result.qr?.provider)
        if (plugin) {
          setActiveQR({ plugin, qr: result.qr })
          toast(`Scan the ${plugin.name} QR code`)
        }
        return
      }
      toast("Connection didn't return a usable sign-in method", 'danger')
    },
    onError: (error: Error) => {
      toast(`Couldn't start sign-in: ${error.message}`, 'danger')
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
  const connectedAccounts = useMemo(
    () =>
      sortedPlugins.flatMap((plugin) =>
        (plugin.connection?.accounts ?? []).map((account) => ({ plugin, account })),
      ),
    [sortedPlugins],
  )
  const selectedPlugin = sortedPlugins.find((plugin) => plugin.id === selectedPluginID)
  const disconnectAccount = (account: IntegrationConnectionAccount) => {
    const label = accountAddress(account) || account.id
    if (window.confirm(`Disconnect ${label}?`)) disconnect.mutate(account.id)
  }

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
          <>
            <SettingsBlock
              title="Existing connections"
              detail={`${connectedAccounts.length} connected ${connectedAccounts.length === 1 ? 'account' : 'accounts'}`}
            >
              {connectedAccounts.length === 0 ? (
                <p className="rounded-card bg-surface px-3 py-3 text-[13px] text-ink-3">
                  No app accounts are connected yet.
                </p>
              ) : (
                <div className="grid gap-2 md:grid-cols-2">
                  {connectedAccounts.map(({ plugin, account }) => (
                    <ExistingConnectionCard
                      key={account.id}
                      plugin={plugin}
                      account={account}
                      disconnecting={disconnect.isPending && disconnect.variables === account.id}
                      onDetails={() => setSelectedPluginID(plugin.id)}
                      onDisconnect={() => disconnectAccount(account)}
                    />
                  ))}
                </div>
              )}
            </SettingsBlock>

            <SettingsBlock title="Add new connection" detail={`${sortedPlugins.length} available`}>
              <div className="grid gap-2 md:grid-cols-2">
                {sortedPlugins.map((plugin) => (
                  <ConnectionPluginCard
                    key={plugin.id}
                    plugin={plugin}
                    connecting={connect.isPending && connect.variables === plugin.id}
                    onOpen={() => setSelectedPluginID(plugin.id)}
                  />
                ))}
              </div>
            </SettingsBlock>

            <ConnectionPluginModal
              plugin={selectedPlugin}
              open={Boolean(selectedPlugin)}
              connecting={connect.isPending && connect.variables === selectedPlugin?.id}
              disconnectingAccountID={disconnect.isPending ? disconnect.variables : undefined}
              onClose={() => setSelectedPluginID(null)}
              onConnect={() => selectedPlugin && connect.mutate(selectedPlugin.id)}
              onDisconnect={disconnectAccount}
            />
            <ConnectionQRModal
              plugin={activeQR?.plugin}
              qr={activeQR?.qr}
              status={qrStatus.data}
              loading={qrStatus.isFetching}
              onClose={() => setActiveQR(null)}
            />
          </>
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
