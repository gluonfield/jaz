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

export function adapterRequiredDescription(plugin: IntegrationPlugin): string {
  const requirements = plugin.auth[0]?.requires ?? []
  if (requirements.length > 0) {
    return `Set ${formatList(requirements)} to enable ${authLabel(plugin.auth[0]?.kind)}.`
  }
  return `This integration needs setup before ${authLabel(plugin.auth[0]?.kind)} can start.`
}

function statusLabel(status: string): string {
  if (status === 'adapter_required') return 'Set up'
  return status
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function authLabel(kind?: string): string {
  if (kind === 'session') return 'QR sign-in'
  if (kind === 'oauth') return 'browser sign-in'
  return 'sign-in'
}

function formatList(values: string[]): string {
  if (values.length <= 1) return values[0] ?? ''
  if (values.length === 2) return `${values[0]} and ${values[1]}`
  return `${values.slice(0, -1).join(', ')}, and ${values[values.length - 1]}`
}
