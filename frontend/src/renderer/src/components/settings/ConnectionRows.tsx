import type { ReactNode } from 'react'
import { Loader2, Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import {
  accountAddress,
  accountSyncLabel,
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

export function ConnectedPluginRow({
  plugin,
  adding,
  disconnectingID,
  onOpen,
  onAddAccount,
  onDisconnect,
}: {
  plugin: IntegrationPlugin
  adding: boolean
  disconnectingID?: string
  onOpen: () => void
  onAddAccount: () => void
  onDisconnect: (account: IntegrationConnectionAccount) => void
}) {
  return (
    <div className="flex gap-3 px-3 py-2">
      <span className="mt-2 shrink-0 self-start">
        <PluginIcon plugin={plugin} compact />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex h-6 items-center justify-between gap-3">
          <button
            type="button"
            onClick={onOpen}
            className="min-w-0 cursor-pointer truncate text-[13px] font-medium text-ink"
          >
            {plugin.name}
          </button>
          {plugin.multi_account && pluginCanConnect(plugin) ? (
            <Button size="sm" variant="ghost" disabled={adding} onClick={onAddAccount}>
              {adding ? <Loader2 size={13} className="animate-spin" /> : <Plus size={13} />}
              {pluginActionLabel(plugin, adding)}
            </Button>
          ) : null}
        </div>
        {plugin.connection?.accounts?.map((account) => (
          <AccountLine
            key={account.id}
            plugin={plugin}
            account={account}
            disconnecting={disconnectingID === account.id}
            onDisconnect={() => onDisconnect(account)}
          />
        ))}
      </div>
    </div>
  )
}

function AccountLine({
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
  const label = [address && address !== plugin.name ? address : 'Connected', accountSyncLabel(account)]
    .filter(Boolean)
    .join(' · ')

  return (
    <div className="group/account flex h-6 items-center gap-2 text-[12px] text-ink-2">
      <span className="size-1.5 shrink-0 rounded-full bg-ok" aria-hidden />
      <span className="min-w-0 truncate" title={label}>
        {label}
      </span>
      <Button
        variant="danger"
        size="sm"
        disabled={disconnecting}
        onClick={onDisconnect}
        aria-label={`Disconnect ${plugin.name} ${label}`}
        className={`ml-auto focus-visible:opacity-100 group-hover/account:opacity-100 [@media(hover:none)]:opacity-100 ${disconnecting ? '' : 'opacity-0'}`}
      >
        {disconnecting ? 'Disconnecting…' : 'Disconnect'}
      </Button>
    </div>
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
