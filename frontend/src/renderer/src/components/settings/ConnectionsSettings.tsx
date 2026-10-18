import { useState } from 'react'
import { SearchField } from '@/components/ui/SearchField'
import { SkeletonRows } from '@/components/ui/Skeleton'
import type { IntegrationPlugin } from '@/lib/api/types'
import {
  AvailablePluginRow,
  ConnectedPluginTile,
  ConnectionSection,
} from '@/components/settings/ConnectionRows'
import { ConnectionPluginDetailModal } from '@/components/settings/ConnectionPluginDetailModal'
import { ConnectionQRModal } from '@/components/settings/ConnectionQRModal'
import { accountLabel, categoryLabel } from '@/components/settings/connectionFormatting'
import { useConnectionSignIn } from '@/components/settings/useConnectionSignIn'

export function ConnectionsSettings() {
  const [selectedPluginID, setSelectedPluginID] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const signIn = useConnectionSignIn({ onStartAccepted: () => setSelectedPluginID(null) })
  const plugins = signIn.plugins
  const allPlugins = plugins.data ?? []
  const selectedPlugin = allPlugins.find((plugin) => plugin.id === selectedPluginID) ?? null
  const query = search.trim().toLowerCase()
  const visiblePlugins = allPlugins.filter((plugin) => matchesQuery(query, plugin))
  const connected = visiblePlugins.filter((plugin) => plugin.connection?.status === 'connected')
  const available = visiblePlugins.filter((plugin) => plugin.connection?.status !== 'connected')
  const connectingID = signIn.isConnecting ? signIn.connectingPluginID : undefined

  return (
    <section className="py-4">
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
        ) : allPlugins.length === 0 ? (
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
              {connected.length > 0 ? (
                <ConnectionSection title="Connected" count={connected.length}>
                  <div className="grid grid-cols-1 gap-2 lg:grid-cols-2">
                    {connected.map((plugin) => (
                      <ConnectedPluginTile
                        key={plugin.id}
                        plugin={plugin}
                        onOpen={() => setSelectedPluginID(plugin.id)}
                      />
                    ))}
                  </div>
                </ConnectionSection>
              ) : null}

              {available.length > 0 ? (
                <ConnectionSection title="Available" count={available.length}>
                  <div className="-mx-3 grid grid-cols-1 gap-x-2 lg:grid-cols-2">
                    {available.map((plugin) => (
                      <AvailablePluginRow
                        key={plugin.id}
                        plugin={plugin}
                        connecting={connectingID === plugin.id}
                        onOpen={() => setSelectedPluginID(plugin.id)}
                        onConnect={() => signIn.start(plugin)}
                      />
                    ))}
                  </div>
                </ConnectionSection>
              ) : null}

              {visiblePlugins.length === 0 ? (
                <p className="py-2 text-[13px] text-ink-3">No connections match “{search.trim()}”.</p>
              ) : null}
            </div>

            <ConnectionPluginDetailModal
              plugin={selectedPlugin}
              connecting={connectingID === selectedPlugin?.id}
              onClose={() => setSelectedPluginID(null)}
              onConnect={signIn.start}
            />
            <ConnectionQRModal {...signIn.qrModal} />
          </>
        )}
      </div>
    </section>
  )
}

function matchesQuery(query: string, plugin: IntegrationPlugin): boolean {
  if (!query) return true
  return [
    plugin.name,
    plugin.description ?? '',
    categoryLabel(plugin.category),
    ...(plugin.connection?.accounts ?? []).map(accountLabel),
  ].some((text) => text.toLowerCase().includes(query))
}
