import type { ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import type { IntegrationPlugin } from '@/lib/api/types'
import {
  accountDetail,
  pluginActionLabel,
  pluginCanConnect,
} from '@/components/settings/connectionFormatting'
import { PluginIcon } from '@/components/settings/ConnectionPluginVisuals'

export function ConnectionSection({
  title,
  count,
  children,
}: {
  title: string
  count: number
  children: ReactNode
}) {
  return (
    <section>
      <p className="mb-2 flex items-center gap-2 text-[13px] font-medium text-ink">
        {title}
        <span className="tabular-nums text-ink-3">{count}</span>
      </p>
      {children}
    </section>
  )
}

export function ConnectedPluginTile({ plugin, onOpen }: { plugin: IntegrationPlugin; onOpen: () => void }) {
  const summary =
    (plugin.connection?.accounts ?? [])
      .map((account) => accountDetail(plugin, account))
      .filter(Boolean)
      .join(', ') || 'Connected'

  return (
    <button
      type="button"
      onClick={onOpen}
      className="flex min-w-0 cursor-pointer items-center gap-3 rounded-card bg-surface px-3 py-2 text-left transition-colors duration-150 hover:bg-surface-2"
    >
      <PluginIcon plugin={plugin} compact />
      <span className="min-w-0">
        <span className="block truncate text-[13px] font-medium text-ink">{plugin.name}</span>
        <span className="flex min-w-0 items-center gap-1.5 text-[12px] leading-5 text-ink-2">
          <span className="size-1.5 shrink-0 rounded-full bg-ok" aria-hidden />
          <span className="truncate" title={summary}>
            {summary}
          </span>
        </span>
      </span>
    </button>
  )
}

export function AvailablePluginRow({
  plugin,
  connecting,
  onOpen,
  onConnect,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  onOpen: () => void
  onConnect: () => void
}) {
  return (
    <div className="flex items-center gap-3 rounded-card px-3 py-2 transition-colors duration-150 hover:bg-surface">
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 cursor-pointer items-center gap-3 text-left"
      >
        <PluginIcon plugin={plugin} compact />
        <span className="min-w-0">
          <span className="block truncate text-[13px] font-medium text-ink">{plugin.name}</span>
          {plugin.description ? (
            <span className="block truncate text-[12px] leading-5 text-ink-3" title={plugin.description}>
              {plugin.description}
            </span>
          ) : null}
        </span>
      </button>
      {pluginCanConnect(plugin) ? (
        <Button
          variant="primary"
          size="sm"
          disabled={connecting}
          onClick={onConnect}
          aria-label={`Connect ${plugin.name}`}
        >
          {connecting ? <Loader2 size={13} className="animate-spin" /> : null}
          {pluginActionLabel(plugin, connecting)}
        </Button>
      ) : (
        <span className="shrink-0 text-[12px] text-ink-3">{pluginActionLabel(plugin, false)}</span>
      )}
    </div>
  )
}
