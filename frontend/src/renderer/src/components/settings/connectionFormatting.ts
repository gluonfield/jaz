import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import { hasTime, relativeTime } from '@/lib/format/time'

export function accountAddress(account: IntegrationConnectionAccount): string {
  if (account.account_name) return account.account_name
  if (account.account_id) return account.account_id
  if (account.alias && account.alias !== 'default') return account.alias
  return ''
}

// Never-empty display identity for an account; accountAddress keeps its
// empty-string contract for callers that hide unlabelable accounts.
export function accountLabel(account: IntegrationConnectionAccount): string {
  return accountAddress(account) || account.id
}

// An MCP connection records the server it reaches, not who signed in to it.
export function accountDetail(plugin: IntegrationPlugin, account: IntegrationConnectionAccount): string {
  return plugin.auth[0]?.kind === 'mcp_connection' ? '' : accountAddress(account)
}

export function accountDetails(plugin: IntegrationPlugin): string[] {
  return (plugin.connection?.accounts ?? [])
    .map((account) => accountDetail(plugin, account))
    .filter(Boolean)
}

export function accountsSummary(plugin: IntegrationPlugin): string {
  return accountDetails(plugin).join(', ')
}

export function accountSyncLabel(account: IntegrationConnectionAccount): string {
  if (!hasTime(account.last_synced_at)) return ''
  const value = relativeTime(account.last_synced_at)
  if (!value) return ''
  if (value === 'now') return 'Synced just now'
  if (/^\d+[mhd]$/.test(value)) return `Synced ${value} ago`
  return `Synced ${value}`
}

export function pluginActionLabel(plugin: IntegrationPlugin, connecting: boolean): string {
  if (connecting) return 'Connecting'
  const remoteMCPAuth = plugin.auth[0]?.kind === 'remote_mcp'
  if (!pluginCanConnect(plugin)) return titleCase(plugin.implementation.status)
  if (remoteMCPAuth) return 'Add MCP server'
  if (plugin.connection?.status === 'connected') return plugin.multi_account ? 'Add account' : 'Reconnect'
  return 'Connect'
}

export function pluginCanConnect(plugin: IntegrationPlugin): boolean {
  return plugin.implementation.status === 'available'
}

export function categoryLabel(value?: string): string {
  return value ? titleCase(value) : 'Integration'
}

export function titleCase(value: string): string {
  return value
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
