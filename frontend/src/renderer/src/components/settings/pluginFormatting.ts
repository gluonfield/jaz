import type { IntegrationConnectionAccount, Plugin } from '@/lib/api/plugins'

export function pluginCanConnect(plugin: Plugin): boolean {
  const auth = plugin.auth
  if (!auth) return false
  if (auth.status === 'local_sign_in_required' || auth.status === 'unavailable') return false
  if (auth.connected && !auth.multi_account) return false
  return auth.kind === 'oauth' || auth.kind === 'session' || auth.kind === 'mcp_oauth' || auth.kind === 'mcp_connection'
}

export function pluginActionLabel(plugin: Plugin, connecting: boolean): string {
  const auth = plugin.auth
  if (!auth) return 'Connect'
  if (connecting) return auth.kind === 'session' ? 'Getting QR…' : 'Connecting…'
  if (auth.connected) return auth.multi_account ? 'Add account' : 'Connected'
  if (auth.status === 'local_sign_in_required') return 'Local sign-in required'
  if (auth.kind === 'session') return 'Link with QR'
  return 'Connect'
}

export function accountAddress(account: IntegrationConnectionAccount): string {
  return account.account_id || account.account_name || account.alias || account.id
}

export function accountLabel(account: IntegrationConnectionAccount): string {
  const parts = [account.alias, account.account_name, account.account_id].filter(Boolean)
  const unique = Array.from(new Set(parts))
  return unique[0] ?? account.id
}

export function accountDetail(account: IntegrationConnectionAccount): string {
  const label = accountLabel(account)
  const secondary = [account.account_id, account.account_name].find((value) => value && value !== label)
  return secondary ?? ''
}

export function accountsSummary(plugin: Plugin): string {
  const accounts = plugin.auth?.accounts ?? []
  if (accounts.length === 0) return ''
  return accounts.map(accountLabel).filter(Boolean).join(', ')
}

export function pluginSummary(plugin: Plugin): string {
  const parts: string[] = []
  const accounts = plugin.auth?.accounts ?? []
  if (accounts.length > 0) {
    parts.push(accounts.length === 1 ? accountAddress(accounts[0]) : `${accounts.length} accounts`)
  }
  const skillCount = plugin.skills?.length ?? 0
  if (skillCount > 0) {
    parts.push(`${skillCount} ${skillCount === 1 ? 'skill' : 'skills'}`)
  }
  const mcpCount = plugin.mcp_servers?.length ?? 0
  if (mcpCount > 0) {
    parts.push(`${mcpCount} MCP`)
  }
  const viewCount = plugin.views?.length ?? 0
  if (viewCount > 0) {
    parts.push(`${viewCount} ${viewCount === 1 ? 'view' : 'views'}`)
  }
  if (parts.length > 0) return parts.join(' · ')
  return plugin.description || ''
}

export function titleCase(value: string): string {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (ch) => ch.toUpperCase())
}

export function categoryLabel(category?: string): string {
  if (!category) return 'Other'
  return titleCase(category)
}
