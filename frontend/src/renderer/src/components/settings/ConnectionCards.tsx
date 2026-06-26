import { Clock3, Loader2, Plug, Plus, Unplug } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { SettingsCard } from './SettingsCard'
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
    <section>
      <p className="mb-2 text-[12px] font-medium text-ink-2">{title}</p>
      <SettingsCard className="overflow-hidden">{children}</SettingsCard>
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
    <div className="grid grid-cols-1 gap-3 bg-surface px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <ConnectionSummary plugin={plugin} title={plugin.name} detail={address || account.id} />
      <Button
        variant="danger"
        size="sm"
        disabled={disconnecting}
        onClick={onDisconnect}
        className="md:justify-self-end"
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
    <div className="grid grid-cols-1 gap-3 bg-surface px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <ConnectionSummary plugin={plugin} title={plugin.name} detail={plugin.description} />
      <Button
        variant="secondary"
        size="sm"
        disabled={!available || connecting}
        onClick={onConnect}
        className="md:justify-self-end"
      >
        <Icon size={13} className={connecting ? 'animate-spin' : undefined} />
        {pluginActionLabel(plugin, connecting)}
      </Button>
    </div>
  )
}

function ConnectionSummary({
  plugin,
  title,
  detail,
}: {
  plugin: IntegrationPlugin
  title: string
  detail?: string
}) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <PluginIcon plugin={plugin} compact />
      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-ink" title={title}>
          {title}
        </p>
        {detail ? (
          <p className="mt-0.5 line-clamp-2 text-[12px] leading-5 text-ink-2" title={detail}>
            {detail}
          </p>
        ) : null}
      </div>
    </div>
  )
}
