import { ChevronRight, Loader2, Unplug } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { SettingsCard } from './SettingsCard'
import { accountAddress, accountSyncLabel } from './connectionFormatting'
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
      <p className="mb-2 text-[12px] font-medium text-ink-2">{title}</p>
      <div className="grid grid-cols-1 gap-2">{children}</div>
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
  const sync = accountSyncLabel(account)
  const detail = address || account.id
  const maskDetail = shouldMaskAccountDetail(plugin, account)

  return (
    <SettingsCard className="grid h-full grid-cols-1 gap-3 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
      <ConnectionSummary
        plugin={plugin}
        title={plugin.name}
        detail={maskDetail ? <MaskedAccountDetail value={detail} /> : detail}
        detailTitle={maskDetail ? 'Account hidden until hover or focus' : detail}
        meta={sync}
      />
      <Button
        variant="danger"
        size="sm"
        disabled={disconnecting}
        onClick={onDisconnect}
        className="sm:justify-self-end"
      >
        {disconnecting ? <Loader2 size={13} className="animate-spin" /> : <Unplug size={13} />}
        {disconnecting ? 'Disconnecting' : 'Disconnect'}
      </Button>
    </SettingsCard>
  )
}

function MaskedAccountDetail({ value }: { value: string }) {
  return (
    <span
      tabIndex={0}
      aria-label={value}
      className="group/account inline-grid max-w-full cursor-default align-baseline tabular-nums focus-visible:rounded-[4px] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
    >
      <span className="col-start-1 row-start-1 truncate font-mono tracking-[0.1em] text-ink-3 transition-opacity duration-150 group-hover/account:opacity-0 group-focus/account:opacity-0">
        ••••••••••
      </span>
      <span className="col-start-1 row-start-1 truncate opacity-0 transition-opacity duration-150 group-hover/account:opacity-100 group-focus/account:opacity-100">
        {value}
      </span>
    </span>
  )
}

function shouldMaskAccountDetail(plugin: IntegrationPlugin, account: IntegrationConnectionAccount) {
  return plugin.id === 'telegram' || plugin.id === 'whatsapp' || account.provider === 'telegram' || account.provider === 'whatsapp'
}

export function ConnectionPluginCard({
  plugin,
  onOpen,
}: {
  plugin: IntegrationPlugin
  onOpen: () => void
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="group grid h-full w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-card bg-surface px-3 py-3 text-left transition-colors duration-150 hover:bg-surface-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
    >
      <ConnectionSummary
        plugin={plugin}
        title={plugin.name}
        detail={plugin.description}
        detailTitle={plugin.description}
      />
      <ChevronRight
        size={14}
        className="shrink-0 text-ink-3 transition-transform duration-150 group-hover:translate-x-0.5 group-hover:text-ink-2"
      />
    </button>
  )
}

function ConnectionSummary({
  plugin,
  title,
  detail,
  detailTitle,
  meta,
}: {
  plugin: IntegrationPlugin
  title: string
  detail?: ReactNode
  detailTitle?: string
  meta?: string
}) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <PluginIcon plugin={plugin} compact />
      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-ink" title={title}>
          {title}
        </p>
        {detail ? (
          <p className="mt-0.5 line-clamp-2 text-[12px] leading-5 text-ink-2" title={detailTitle}>
            {detail}
          </p>
        ) : null}
        {meta ? (
          <p className="mt-0.5 truncate text-[11px] leading-4 text-ink-3" title={meta}>
            {meta}
          </p>
        ) : null}
      </div>
    </div>
  )
}
