import { ChevronRight, Clock3, Loader2, Plug, Plus, Unplug } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import {
  accountAddress,
  accountAlias,
  CAPABILITY_LABELS,
  plural,
  pluginActionLabel,
} from './connectionFormatting'
import { Pill, PluginIcon } from './ConnectionPluginVisuals'

export function SettingsBlock({
  title,
  detail,
  children,
}: {
  title: string
  detail?: string
  children: ReactNode
}) {
  return (
    <section className="mt-5">
      <div className="mb-2 flex items-baseline justify-between gap-3">
        <h3 className="text-[13px] font-semibold text-ink">{title}</h3>
        {detail ? <span className="shrink-0 text-[12px] text-ink-3">{detail}</span> : null}
      </div>
      {children}
    </section>
  )
}

export function ExistingConnectionCard({
  plugin,
  account,
  disconnecting,
  onDetails,
  onDisconnect,
}: {
  plugin: IntegrationPlugin
  account: IntegrationConnectionAccount
  disconnecting: boolean
  onDetails: () => void
  onDisconnect: () => void
}) {
  const toolCount = plugin.tools?.length ?? 0
  const address = accountAddress(account)
  const alias = accountAlias(account)

  return (
    <div className="rounded-card bg-surface px-3 py-3 text-[13px] text-ink-2">
      <div className="flex min-w-0 items-start gap-3">
        <PluginIcon plugin={plugin} />
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium text-ink" title={plugin.name}>
            {plugin.name}
          </p>
          {address ? (
            <p className="mt-0.5 truncate text-[12px] text-ink-3" title={address}>
              {address}
            </p>
          ) : null}
          <div className="mt-2 flex flex-wrap gap-1.5">
            {alias ? <Pill>{alias}</Pill> : null}
            <Pill>{toolCount > 0 ? `${toolCount} ${plural(toolCount, 'tool')}` : 'No tools'}</Pill>
            <Pill>{plugin.provider.name}</Pill>
          </div>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap justify-end gap-1.5">
        <Button variant="secondary" size="sm" onClick={onDetails} className="min-h-10">
          Details
        </Button>
        <Button
          variant="danger"
          size="sm"
          disabled={disconnecting}
          onClick={onDisconnect}
          className="min-h-10"
        >
          {disconnecting ? <Loader2 size={13} className="animate-spin" /> : <Unplug size={13} />}
          {disconnecting ? 'Disconnecting' : 'Disconnect'}
        </Button>
      </div>
    </div>
  )
}

export function ConnectionPluginCard({
  plugin,
  connecting,
  onOpen,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  onOpen: () => void
}) {
  const available = plugin.implementation.status === 'available'
  const connected = plugin.connection?.status === 'connected'
  const accountCount = plugin.connection?.accounts?.length ?? 0

  return (
    <button
      type="button"
      onClick={onOpen}
      className="group flex min-h-[92px] w-full cursor-pointer items-start gap-3 rounded-card px-3 py-3 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface focus-visible:bg-surface focus-visible:outline-none"
    >
      <PluginIcon plugin={plugin} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-1.5">
          <span className="truncate font-medium text-ink" title={plugin.name}>
            {plugin.name}
          </span>
          {plugin.multi_account ? <Pill>Multi-account</Pill> : null}
        </div>
        {plugin.description ? (
          <p className="mt-1 line-clamp-2 text-[12px] leading-5 text-ink-3">
            {plugin.description}
          </p>
        ) : null}
        <div className="mt-2 flex flex-wrap gap-1.5">
          {plugin.capabilities.slice(0, 3).map((capability) => (
            <Pill key={capability}>{CAPABILITY_LABELS[capability]}</Pill>
          ))}
          {accountCount > 0 ? <Pill>{`${accountCount} connected`}</Pill> : null}
        </div>
      </div>
      <span className="mt-0.5 inline-flex min-h-10 shrink-0 items-center gap-1.5 rounded-full bg-surface-2 px-2.5 text-[12px] font-medium text-ink-2">
        {connecting ? (
          <Loader2 size={13} className="animate-spin" />
        ) : available ? (
          connected && plugin.multi_account ? (
            <Plus size={13} />
          ) : (
            <Plug size={13} />
          )
        ) : (
          <Clock3 size={13} />
        )}
        {pluginActionLabel(plugin, connecting)}
        <ChevronRight size={13} className="-mr-1 text-ink-3 transition-transform duration-150 group-hover:translate-x-0.5" />
      </span>
    </button>
  )
}
