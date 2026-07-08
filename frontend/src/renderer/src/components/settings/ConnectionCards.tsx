import { Loader2, Unplug } from 'lucide-react'
import type { ReactNode } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { accountAddress, accountSyncLabel } from './connectionFormatting'
import { PluginIcon } from './ConnectionPluginVisuals'

// Shared by the section grids and the loading skeleton so they can't drift.
// lg (not sm) because the settings pane is viewport minus a 272px sidebar:
// three columns only fit once the window is genuinely wide.
export const connectionsGridClass = 'grid grid-cols-2 gap-2 lg:grid-cols-3'

const tileClass =
  'flex h-full w-full cursor-pointer flex-col items-center justify-center rounded-card bg-surface px-3 py-5 text-center transition-[background-color,scale] duration-150 hover:bg-surface-2 active:scale-[0.97]'

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
  onOpen,
  onDisconnect,
}: {
  plugin: IntegrationPlugin
  account: IntegrationConnectionAccount
  disconnecting: boolean
  onOpen: () => void
  onDisconnect: () => void
}) {
  const detail = accountAddress(account) || account.id
  const sync = accountSyncLabel(account)

  return (
    <div className="group relative h-full">
      <button type="button" onClick={onOpen} className={tileClass}>
        <TileIdentity plugin={plugin} />
        <p className="mt-0.5 w-full truncate text-[12px] leading-5 text-ink-2" title={detail}>
          {detail}
        </p>
        {sync ? (
          <p className="mt-0.5 w-full truncate text-[11px] leading-4 text-ink-3" title={sync}>
            {sync}
          </p>
        ) : null}
      </button>
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
    </div>
  )
}

export function ConnectionPluginCard({
  plugin,
  onOpen,
}: {
  plugin: IntegrationPlugin
  onOpen: () => void
}) {
  return (
    <button type="button" onClick={onOpen} title={plugin.description} className={tileClass}>
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
