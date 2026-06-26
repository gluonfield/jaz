import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { hasTime, relativeTime } from '@/lib/format/time'

export function accountAddress(account: IntegrationConnectionAccount): string {
  if (account.account_id) return account.account_id
  if (account.account_name) return account.account_name
  if (account.alias && account.alias !== 'default') return account.alias
  return ''
}

export function accountSyncLabel(account: IntegrationConnectionAccount): string {
  if (!hasTime(account.last_synced_at)) return ''
  const value = relativeTime(account.last_synced_at)
  return value ? `Synced ${value}` : ''
}

export function pluginActionLabel(plugin: IntegrationPlugin, connecting: boolean): string {
  if (connecting) return 'Connecting'
  const sessionAuth = plugin.auth[0]?.kind === 'session'
  if (!pluginCanConnect(plugin)) return sessionAuth ? 'Unavailable' : statusLabel(plugin.implementation.status)
  if (plugin.connection?.status === 'connected' && plugin.multi_account) return 'Add account'
  if (plugin.connection?.status === 'connected') return 'Reconnect'
  if (sessionAuth) return 'QR sign in'
  return 'Connect'
}

export function pluginCanConnect(plugin: IntegrationPlugin): boolean {
  return plugin.implementation.status === 'available'
}

function statusLabel(status: string): string {
  return status
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
