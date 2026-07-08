import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import {
  accountAddress,
  accountSyncLabel,
  pluginActionLabel,
  pluginCanConnect,
} from './connectionFormatting'
import { PluginIcon } from './ConnectionPluginVisuals'

export function ConnectionSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section>
      <p className="border-b border-border/60 pb-2 text-[13px] font-medium text-ink">{title}</p>
      <div className="-mx-3 mt-1 grid grid-cols-1 gap-x-2 lg:grid-cols-2">{children}</div>
    </section>
  )
}

export function ConnectedAccountRow({
  plugin,
  account,
  disconnecting,
  onOpen,
  onDisconnect,
}: {
  plugin: IntegrationPlugin
  account: IntegrationConnectionAccount
  disconnecting: boolean
  onOpen: () => void
  onDisconnect: () => void
}) {
  const address = accountAddress(account) || account.id
  const subtitle = [address, accountSyncLabel(account)].filter(Boolean).join(' · ')

  return (
    <ConnectionRow
      plugin={plugin}
      subtitle={subtitle}
      onOpen={onOpen}
      action={
        <Button
          variant="danger"
          size="sm"
          className="ring-1 ring-border"
          disabled={disconnecting}
          onClick={onDisconnect}
        >
          {disconnecting ? 'Disconnecting' : 'Disconnect'}
        </Button>
      }
    />
  )
}

export function PluginCatalogRow({
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
    <ConnectionRow
      plugin={plugin}
      subtitle={plugin.description}
      onOpen={onOpen}
      action={
        <Button
          variant="secondary"
          size="sm"
          className="ring-1 ring-border"
          disabled={!pluginCanConnect(plugin) || connecting}
          onClick={onConnect}
        >
          {pluginActionLabel(plugin, connecting)}
        </Button>
      }
    />
  )
}

function ConnectionRow({
  plugin,
  subtitle,
  onOpen,
  action,
}: {
  plugin: IntegrationPlugin
  subtitle?: string
  onOpen: () => void
  action: ReactNode
}) {
  return (
    <div className="flex h-full items-center gap-3 rounded-card px-3 py-2.5 transition-colors duration-150 hover:bg-surface">
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 cursor-pointer items-center gap-3 text-left"
      >
        <PluginIcon plugin={plugin} />
        <span className="min-w-0">
          <span className="block truncate text-[13px] font-medium text-ink">{plugin.name}</span>
          {subtitle ? (
            <span className="block truncate text-[12px] leading-5 text-ink-2" title={subtitle}>
              {subtitle}
            </span>
          ) : null}
        </span>
      </button>
      <span className="shrink-0">{action}</span>
    </div>
  )
}
