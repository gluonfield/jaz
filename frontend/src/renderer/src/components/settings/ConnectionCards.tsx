import { Loader2, Unplug } from 'lucide-react'
import type { ReactNode } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { SettingsCard } from './SettingsCard'
import { accountAddress, accountSyncLabel } from './connectionFormatting'
import { PluginIcon } from './ConnectionPluginVisuals'

// Shared by the section grids and the loading skeleton so they can't drift.
// lg (not sm) because the settings pane is viewport minus a 272px sidebar:
// three columns only fit once the window is genuinely wide.
export const connectionsGridClass = 'grid grid-cols-2 gap-2 lg:grid-cols-3'

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
      <div className={connectionsGridClass}>{children}</div>
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
  const detail = accountAddress(account) || account.id
  const sync = accountSyncLabel(account)
  const maskDetail = shouldMaskAccountDetail(plugin, account)

  return (
    <SettingsCard className="group relative flex h-full flex-col items-center px-3 pb-4 pt-5 text-center">
      <span
        className={`absolute right-1 top-1 transition-opacity duration-150 ${disconnecting ? '' : 'opacity-0 focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100'}`}
      >
        <IconButton
          variant="danger"
          aria-label={`Disconnect ${plugin.name} ${detail}`}
          disabled={disconnecting}
          onClick={onDisconnect}
        >
          {disconnecting ? <Loader2 size={14} className="animate-spin" /> : <Unplug size={14} />}
        </IconButton>
      </span>
      <TileIdentity plugin={plugin} />
      <p
        className="mt-0.5 w-full truncate text-[12px] leading-5 text-ink-2"
        title={maskDetail ? undefined : detail}
      >
        {maskDetail ? <MaskedAccountDetail value={detail} /> : detail}
      </p>
      {sync ? (
        <p className="mt-0.5 w-full truncate text-[11px] leading-4 text-ink-3" title={sync}>
          {sync}
        </p>
      ) : null}
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
      title={plugin.description}
      className="flex h-full w-full cursor-pointer flex-col items-center rounded-card bg-surface px-3 py-5 text-center transition-[background-color,scale] duration-150 hover:bg-surface-2 active:scale-[0.97]"
    >
      <TileIdentity plugin={plugin} />
    </button>
  )
}

function TileIdentity({ plugin }: { plugin: IntegrationPlugin }) {
  return (
    <>
      <PluginIcon plugin={plugin} />
      <p className="mt-2.5 w-full truncate text-[13px] font-medium text-ink" title={plugin.name}>
        {plugin.name}
      </p>
    </>
  )
}
