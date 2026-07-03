import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { AnimatedList, AnimatedListItem } from '@/components/ui/AnimatedList'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { disconnectConnectionAccount, updateConnectionAccountScopes } from '@/lib/api/connections'
import { keys } from '@/lib/query/keys'
import type { IntegrationConnectionAccount } from '@/lib/api/types'
import {
  ConnectionSection,
  ConnectionPluginCard,
  ExistingConnectionCard,
} from './ConnectionCards'
import { ConnectionPluginDetailModal } from './ConnectionPluginDetailModal'
import { ConnectionQRModal } from './ConnectionQRModal'
import { accountAddress } from './connectionFormatting'
import { useConnectionSignIn } from './useConnectionSignIn'

export function ConnectionsSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [selectedPluginID, setSelectedPluginID] = useState<string | null>(null)
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
  const updateScopes = useMutation({
    mutationFn: ({ id, scopes }: { id: string; scopes: string[] }) =>
      updateConnectionAccountScopes(id, scopes),
    onError: (error: Error) => toast(`Couldn't update access: ${error.message}`, 'danger'),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.connectionPlugins }),
  })
  const connectedAccounts = useMemo(
    () =>
      sortedPlugins.flatMap((plugin) =>
        (plugin.connection?.accounts ?? []).map((account) => ({ plugin, account })),
      ),
    [sortedPlugins],
  )
  const hasConnectedAccounts = connectedAccounts.length > 0
  const hasCatalogPlugins = sortedPlugins.length > 0
  const hasVisibleConnections = hasConnectedAccounts || hasCatalogPlugins
  const disconnectAccount = (account: IntegrationConnectionAccount) => {
    const label = accountAddress(account) || account.id
    if (window.confirm(`Disconnect ${label}?`)) disconnect.mutate(account.id)
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
          <SkeletonRows count={3} />
        ) : plugins.isError ? (
          <p className="py-2 text-[13px] text-danger">{plugins.error.message}</p>
        ) : !hasVisibleConnections ? (
          <p className="rounded-card bg-surface px-3 py-3 text-[13px] text-ink-3">
            No first-party connections are available yet.
          </p>
        ) : (
          <>
            <div className="space-y-5">
              {hasConnectedAccounts ? (
                <ConnectionSection title="Existing connections">
                  <AnimatedList>
                    {connectedAccounts.map(({ plugin, account }) => (
                      <AnimatedListItem key={account.id}>
                        <ExistingConnectionCard
                          plugin={plugin}
                          account={account}
                          disconnecting={disconnect.isPending && disconnect.variables === account.id}
                          updatingScopes={updateScopes.isPending && updateScopes.variables?.id === account.id}
                          onScopesChange={(scopes) =>
                            updateScopes.mutate({ id: account.id, scopes })
                          }
                          onDisconnect={() => disconnectAccount(account)}
                        />
                      </AnimatedListItem>
                    ))}
                  </AnimatedList>
                </ConnectionSection>
              ) : null}

              {hasCatalogPlugins ? (
                <ConnectionSection title="Add connection">
                  <AnimatedList>
                    {sortedPlugins.map((plugin) => (
                      <AnimatedListItem key={plugin.id}>
                        <ConnectionPluginCard
                          plugin={plugin}
                          onOpen={() => setSelectedPluginID(plugin.id)}
                        />
                      </AnimatedListItem>
                    ))}
                  </AnimatedList>
                </ConnectionSection>
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
