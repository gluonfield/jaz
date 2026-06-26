import { Loader2, Plug, Plus, Unplug } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { accountAddress, pluginActionLabel } from './connectionFormatting'
import { PluginGlyph } from './ConnectionPluginVisuals'

export function ConnectionPluginDetailModal({
  plugin,
  connecting,
  disconnectingAccountID,
  onClose,
  onConnect,
  onDisconnect,
}: {
  plugin: IntegrationPlugin | null
  connecting: boolean
  disconnectingAccountID?: string
  onClose: () => void
  onConnect: (plugin: IntegrationPlugin) => void
  onDisconnect: (account: IntegrationConnectionAccount) => void
}) {
  if (!plugin) return null

  const accounts = plugin.connection?.accounts ?? []
  const available = plugin.implementation.status === 'available'
  const connected = plugin.connection?.status === 'connected'
  const ConnectIcon = connecting ? Loader2 : available && connected && plugin.multi_account ? Plus : Plug

  return (
    <Modal
      open
      onClose={onClose}
      title={plugin.name}
      description={plugin.description}
      icon={<PluginGlyph plugin={plugin} size={18} />}
      size="md"
      footer={
        <>
          <p className="min-w-0 truncate text-[12px] text-ink-3">
            {available ? '' : connectionStatus(plugin.implementation.status)}
          </p>
          <Button
            variant="primary"
            size="md"
            disabled={!available || connecting}
            onClick={() => onConnect(plugin)}
          >
            <ConnectIcon size={14} className={connecting ? 'animate-spin' : undefined} />
            {pluginActionLabel(plugin, connecting)}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <ConnectionDetails plugin={plugin} />
        {accounts.length ? (
          <section>
            <p className="mb-2 text-[12px] font-medium text-ink-2">Connected accounts</p>
            <div className="space-y-1">
              {accounts.map((account) => (
                <ConnectedAccountRow
                  key={account.id}
                  account={account}
                  disconnecting={disconnectingAccountID === account.id}
                  onDisconnect={() => onDisconnect(account)}
                />
              ))}
            </div>
          </section>
        ) : null}
      </div>
    </Modal>
  )
}

function ConnectionDetails({ plugin }: { plugin: IntegrationPlugin }) {
  const auth = plugin.auth[0]
  return (
    <dl className="grid grid-cols-[92px_minmax(0,1fr)] gap-x-3 gap-y-2 rounded-card bg-surface px-3 py-3 text-[13px]">
      <dt className="text-ink-3">Provider</dt>
      <dd className="min-w-0 truncate text-ink">{plugin.provider.name}</dd>
      <dt className="text-ink-3">Accounts</dt>
      <dd className="text-ink">{plugin.multi_account ? 'Multiple accounts' : 'One account'}</dd>
      {auth ? (
        <>
          <dt className="text-ink-3">Sign in</dt>
          <dd className="text-ink">{authDescription(auth.kind)}</dd>
        </>
      ) : null}
    </dl>
  )
}

function ConnectedAccountRow({
  account,
  disconnecting,
  onDisconnect,
}: {
  account: IntegrationConnectionAccount
  disconnecting: boolean
  onDisconnect: () => void
}) {
  const address = accountAddress(account) || account.id
  const name = account.account_name && account.account_name !== address ? account.account_name : ''

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-card bg-surface px-3 py-2">
      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-ink" title={address}>
          {address}
        </p>
        {name ? (
          <p className="mt-0.5 truncate text-[12px] text-ink-3" title={name}>
            {name}
          </p>
        ) : null}
      </div>
      <Button variant="danger" size="sm" disabled={disconnecting} onClick={onDisconnect}>
        {disconnecting ? <Loader2 size={13} className="animate-spin" /> : <Unplug size={13} />}
        Disconnect
      </Button>
    </div>
  )
}

function authDescription(kind: string): string {
  switch (kind) {
    case 'oauth':
      return 'Browser sign-in'
    case 'session':
      return 'QR pairing'
    case 'remote_mcp':
      return 'Remote MCP'
    case 'browser_local':
      return 'Local browser'
    default:
      return connectionStatus(kind)
  }
}

function connectionStatus(status: string): string {
  return status
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
