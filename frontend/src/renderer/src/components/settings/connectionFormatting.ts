import type {
  IntegrationCapability,
  IntegrationConnectionAccount,
  IntegrationPlugin,
} from '@/lib/api/types'

export const CAPABILITY_LABELS: Record<IntegrationCapability, string> = {
  sync: 'Sync',
  act: 'Actions',
  materialize: 'Memory',
  mcp: 'MCP',
  browser: 'Browser',
}

export function accountAddress(account: IntegrationConnectionAccount): string {
  if (account.account_id) return account.account_id
  if (account.account_name) return account.account_name
  if (account.alias && account.alias !== 'default') return account.alias
  return ''
}

export function accountAlias(account: IntegrationConnectionAccount): string {
  const address = accountAddress(account)
  if (account.alias && account.alias !== 'default' && account.alias !== address) return account.alias
  if (account.account_name && account.account_name !== address) return account.account_name
  return ''
}

export function pluginActionLabel(plugin: IntegrationPlugin, connecting: boolean): string {
  if (connecting) return 'Connecting'
  if (plugin.implementation.status !== 'available') return statusLabel(plugin.implementation.status)
  if (plugin.connection?.status === 'connected' && plugin.multi_account) return 'Add account'
  if (plugin.connection?.status === 'connected') return 'Reconnect'
  return 'Connect'
}

export function statusLabel(status: string): string {
  return status
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

export function plural(count: number, word: string): string {
  return count === 1 ? word : `${word}s`
}
