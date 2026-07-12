import type { ReactNode } from 'react'
import { Link2, Loader2, Unlink2 } from 'lucide-react'
import { IconButton } from '@/components/ui/IconButton'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import {
  accountLabel,
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
  const subtitle = [accountLabel(account), accountSyncLabel(account)].filter(Boolean).join(' · ')
  const actionLabel = disconnecting ? 'Disconnecting' : 'Disconnect'

  return (
    <ConnectionRow
      plugin={plugin}
      subtitle={subtitle}
      onOpen={onOpen}
      action={
        <IconButton
          variant="danger"
          size="sm"
          className="relative text-ink-2 after:absolute after:-inset-1.5 after:content-['']"
          disabled={disconnecting}
          onClick={onDisconnect}
          aria-label={`${actionLabel} ${plugin.name}`}
          title={actionLabel}
        >
          {disconnecting ? (
            <Loader2 size={14} className="animate-spin" aria-hidden />
          ) : (
            <Unlink2 size={14} aria-hidden />
          )}
        </IconButton>
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
  const actionLabel = pluginActionLabel(plugin, connecting)

  return (
    <ConnectionRow
      plugin={plugin}
      subtitle={plugin.description}
      onOpen={onOpen}
      action={
        <IconButton
          size="sm"
          className="relative text-ink-2 after:absolute after:-inset-1.5 after:content-['']"
          disabled={!pluginCanConnect(plugin) || connecting}
          onClick={onConnect}
          aria-label={`${actionLabel} ${plugin.name}`}
          title={actionLabel}
        >
          {connecting ? (
            <Loader2 size={14} className="animate-spin" aria-hidden />
          ) : (
            <Link2 size={14} aria-hidden />
          )}
        </IconButton>
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
    <div className="flex items-center gap-3 rounded-card px-3 py-2 transition-colors duration-150 hover:bg-surface">
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
