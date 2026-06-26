import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'

export function accountAddress(account: IntegrationConnectionAccount): string {
  if (account.account_id) return account.account_id
  if (account.account_name) return account.account_name
  if (account.alias && account.alias !== 'default') return account.alias
  return ''
}

export function pluginActionLabel(plugin: IntegrationPlugin, connecting: boolean): string {
  if (connecting) return 'Connecting'
  if (plugin.implementation.status !== 'available') return statusLabel(plugin.implementation.status)
  if (plugin.connection?.status === 'connected' && plugin.multi_account) return 'Add account'
  if (plugin.connection?.status === 'connected') return 'Reconnect'
  return 'Connect'
}

function statusLabel(status: string): string {
  if (status === 'adapter_required') return 'Adapter required'
  return status
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
