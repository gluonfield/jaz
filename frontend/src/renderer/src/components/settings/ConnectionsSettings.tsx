import { useQuery } from '@tanstack/react-query'
import { Clock3, Mail } from 'lucide-react'
import { useMemo } from 'react'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { connectionPluginsQuery } from '@/lib/api/connections'
import type { IntegrationCapability, IntegrationPlugin } from '@/lib/api/types'

const CAPABILITY_LABELS: Record<IntegrationCapability, string> = {
  sync: 'Sync',
  act: 'Actions',
  materialize: 'Memory',
  mcp: 'MCP',
  browser: 'Browser',
}

export function ConnectionsSettings() {
  const plugins = useQuery(connectionPluginsQuery)
  const sortedPlugins = useMemo(
    () => [...(plugins.data ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [plugins.data],
  )

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
              <ConnectionPluginRow key={plugin.id} plugin={plugin} />
            ))}
          </div>
        )}
      </div>
    </section>
  )
}

function ConnectionPluginRow({ plugin }: { plugin: IntegrationPlugin }) {
  const toolCount = plugin.tools?.length ?? 0
  const sourceLanes = plugin.source_lanes ?? []

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
        </div>
      </div>

      <span
        className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full bg-surface-2 px-2.5 text-[12px] font-medium text-ink-3 max-sm:self-start"
        title="The first-party account connection flow is not implemented yet."
      >
        <Clock3 size={13} />
        {statusLabel(plugin.implementation.status)}
      </span>
    </div>
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
