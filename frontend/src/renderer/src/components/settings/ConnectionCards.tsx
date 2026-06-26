import { Clock3, Loader2, Plug, Plus, Unplug } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { accountAddress, pluginActionLabel } from './connectionFormatting'
import { PluginIcon } from './ConnectionPluginVisuals'

export function SettingsBlock({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="mt-5">
      <div className="mb-2">
        <h3 className="text-[13px] font-semibold text-ink">{title}</h3>
      </div>
      {children}
    </section>
  )
}

export function ExistingConnectionCard({
  plugin,
  account,
  disconnecting,
  onDisconnect,
}: {
  plugin: IntegrationPlugin
  account: IntegrationConnectionAccount
  disconnecting: boolean
  onDisconnect: () => void
}) {
  const address = accountAddress(account)

  return (
    <div className="flex min-h-[64px] items-center gap-3 rounded-card bg-surface px-3 py-2 text-[13px] text-ink-2">
      <PluginIcon plugin={plugin} compact />
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium text-ink" title={plugin.name}>
          {plugin.name}
        </p>
        <div className="mt-0.5 min-h-5">
          <p className="truncate text-[12px] text-ink-3" title={address || account.id}>
            {address || account.id}
          </p>
        </div>
      </div>
      <Button
        variant="danger"
        size="sm"
        disabled={disconnecting}
        onClick={onDisconnect}
        className="min-h-10 shrink-0"
      >
        {disconnecting ? <Loader2 size={13} className="animate-spin" /> : <Unplug size={13} />}
        {disconnecting ? 'Disconnecting' : 'Disconnect'}
      </Button>
    </div>
  )
}

export function ConnectionPluginCard({
  plugin,
  connecting,
  onConnect,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  onConnect: () => void
}) {
  const available = plugin.implementation.status === 'available'
  const connected = plugin.connection?.status === 'connected'
  const Icon = connecting ? Loader2 : available && connected && plugin.multi_account ? Plus : available ? Plug : Clock3

  return (
    <div className="flex min-h-[72px] items-center gap-3 rounded-card px-3 py-2 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface">
      <PluginIcon plugin={plugin} compact />
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium text-ink" title={plugin.name}>
          {plugin.name}
        </p>
        {plugin.description ? (
          <p className="mt-0.5 line-clamp-1 text-[12px] leading-5 text-ink-3">
            {plugin.description}
          </p>
        ) : null}
      </div>
      <Button
        variant={connected && plugin.multi_account ? 'secondary' : 'primary'}
        size="sm"
        disabled={!available || connecting}
        onClick={onConnect}
        className="min-h-10 shrink-0"
      >
        <Icon size={13} className={connecting ? 'animate-spin' : undefined} />
        {pluginActionLabel(plugin, connecting)}
      </Button>
    </div>
  )
}
