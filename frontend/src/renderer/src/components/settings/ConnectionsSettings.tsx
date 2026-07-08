import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { SearchField } from '@/components/ui/SearchField'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { disconnectConnectionAccount } from '@/lib/api/connections'
import { keys } from '@/lib/query/keys'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import {
  ConnectionSection,
  ConnectedAccountRow,
  PluginCatalogRow,
} from './ConnectionRows'
import { ConnectionPluginDetailModal } from './ConnectionPluginDetailModal'
import { ConnectionQRModal } from './ConnectionQRModal'
import { accountLabel, categoryLabel } from './connectionFormatting'
import { useConnectionSignIn } from './useConnectionSignIn'

export function ConnectionsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [selectedPluginID, setSelectedPluginID] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const signIn = useConnectionSignIn({ onStartAccepted: () => setSelectedPluginID(null) })
  const plugins = signIn.plugins
  const sortedPlugins = useMemo(
    () => [...(plugins.data ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [plugins.data],
  )
  const selectedPlugin = useMemo(
    () => sortedPlugins.find((plugin) => plugin.id === selectedPluginID) ?? null,
    [sortedPlugins, selectedPluginID],
  )
  const disconnect = useMutation({
    mutationFn: disconnectConnectionAccount,
    onSuccess: () => toast('Disconnected account'),
    onError: (error: Error) => toast(`Couldn't disconnect account: ${error.message}`, 'danger'),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.connectionPlugins }),
  })
  const query = search.trim().toLowerCase()
  const connectedAccounts = useMemo(
    () =>
      sortedPlugins.flatMap((plugin) =>
        (plugin.connection?.accounts ?? []).map((account) => ({ plugin, account })),
      ),
    [sortedPlugins],
  )
  const visibleAccounts = connectedAccounts.filter(({ plugin, account }) =>
    matchesQuery(query, plugin, accountLabel(account)),
  )
  const catalogGroups = useMemo(() => {
    const groups = new Map<string, IntegrationPlugin[]>()
    for (const plugin of sortedPlugins) {
      if (!matchesQuery(query, plugin)) continue
      const category = categoryLabel(plugin.category)
      groups.set(category, [...(groups.get(category) ?? []), plugin])
    }
    return [...groups.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([category, items]) => ({ category, items }))
  }, [sortedPlugins, query])
  const disconnectAccount = (account: IntegrationConnectionAccount) => {
    if (window.confirm(`Disconnect ${accountLabel(account)}?`)) disconnect.mutate(account.id)
  }

  return (
    <section className="py-5">
      <div>
        <p className="text-sm font-medium text-ink">Connections</p>
        <p className="mt-0.5 text-[13px] text-ink-2">
          Connect accounts for agent tools and memory.
        </p>
      </div>

      <div className="mt-4">
        {plugins.isPending ? (
          <SkeletonRows count={5} />
        ) : plugins.isError ? (
          <p className="py-2 text-[13px] text-danger">{plugins.error.message}</p>
        ) : sortedPlugins.length === 0 ? (
          <p className="rounded-card bg-surface px-3 py-3 text-[13px] text-ink-3">
            No first-party connections are available yet.
          </p>
        ) : (
          <>
            <SearchField
              value={search}
              onChange={setSearch}
              placeholder="Search connections…"
              className="h-9"
            />

            <div className="mt-5 space-y-6">
              {visibleAccounts.length > 0 ? (
                <ConnectionSection title="Connected">
                  {visibleAccounts.map(({ plugin, account }) => (
                    <ConnectedAccountRow
                      key={account.id}
                      plugin={plugin}
                      account={account}
                      disconnecting={disconnect.isPending && disconnect.variables === account.id}
                      onOpen={() => setSelectedPluginID(plugin.id)}
                      onDisconnect={() => disconnectAccount(account)}
                    />
                  ))}
                </ConnectionSection>
              ) : null}

              {catalogGroups.map(({ category, items }) => (
                <ConnectionSection key={category} title={category}>
                  {items.map((plugin) => (
                    <PluginCatalogRow
                      key={plugin.id}
                      plugin={plugin}
                      connecting={signIn.isConnecting && signIn.connectingPluginID === plugin.id}
                      onOpen={() => setSelectedPluginID(plugin.id)}
                      onConnect={() => signIn.start(plugin)}
                    />
                  ))}
                </ConnectionSection>
              ))}

              {visibleAccounts.length === 0 && catalogGroups.length === 0 ? (
                <p className="py-2 text-[13px] text-ink-3">No connections match “{search.trim()}”.</p>
              ) : null}
            </div>

            <ConnectionPluginDetailModal
              plugin={selectedPlugin}
              connecting={signIn.isConnecting && signIn.connectingPluginID === selectedPlugin?.id}
              onClose={() => setSelectedPluginID(null)}
              onConnect={signIn.start}
            />
            <ConnectionQRModal
              plugin={signIn.activeQR?.plugin}
              qr={signIn.activeQR?.qr}
              status={signIn.qrStatus}
              loading={signIn.qrLoading}
              refreshing={signIn.qrRefreshing}
              passwordSubmitting={signIn.qrPasswordSubmitting}
              onClose={signIn.closeQR}
              onRefresh={signIn.refreshQR}
              onSubmitPassword={signIn.submitQRPassword}
            />
          </>
        )}
      </div>
    </section>
  )
}

function matchesQuery(query: string, plugin: IntegrationPlugin, accountText = ''): boolean {
  if (!query) return true
  return (
    plugin.name.toLowerCase().includes(query) ||
    (plugin.description ?? '').toLowerCase().includes(query) ||
    categoryLabel(plugin.category).toLowerCase().includes(query) ||
    accountText.toLowerCase().includes(query)
  )
}
