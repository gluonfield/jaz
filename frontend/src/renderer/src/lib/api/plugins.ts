import { queryOptions } from '@tanstack/react-query'
import { del, get, post, put } from '@/lib/api/client'
import type { ConnectionQRStart, ConnectionQRStatus, ConnectionStart, IntegrationConnectionAccount, PluginView } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export type PluginIconKind = 'asset' | 'url' | 'data' | 'initials'

export interface PluginIcon {
  kind: PluginIconKind
  value: string
  background?: string
}

export interface PluginSkill {
  name: string
  description?: string
}

export interface PluginMCPServer {
  name: string
  transport: string
}

export interface PluginTool {
  name: string
  description?: string
  risk?: string
}

export interface PluginConfigVariable {
  name: string
  description?: string
  required?: boolean
  secret?: boolean
}

export interface PluginAuth {
  kind: string
  status: string
  reason?: string
  multi_account?: boolean
  connected: boolean
  accounts?: IntegrationConnectionAccount[]
}

export interface PluginSourceRef {
  id?: string
  repository: string
  ref?: string
  path?: string
  commit: string
  remote_commit?: string
  update_available?: boolean
  auto_update?: boolean
}

export interface Plugin {
  id: string
  name: string
  display_name: string
  description?: string
  version?: string
  category?: string
  author?: string
  author_url?: string
  homepage?: string
  repository?: string
  keywords?: string[]
  suggested_prompts?: string[]
  icon: PluginIcon
  icon_data_url?: string
  origin: 'builtin' | 'git' | 'marketplace'
  execution?: 'cloud' | 'device'
  installed: boolean
  enabled: boolean
  capabilities?: string[]
  skills?: PluginSkill[]
  views?: string[]
  mcp_servers?: PluginMCPServer[]
  tools?: PluginTool[]
  config_variables?: PluginConfigVariable[]
  config_set?: Record<string, boolean>
  auth?: PluginAuth
  page?: { view: string }
  source?: PluginSourceRef
  warnings?: string[]
}

export interface PluginSource {
  id: string
  repository: string
  url: string
  ref?: string
  paths?: string[]
  commit_sha?: string
  auto_update?: boolean
  is_marketplace?: boolean
  checked_at?: string
}

export interface PluginCatalog {
  plugins: Plugin[]
  sources: PluginSource[]
}

export interface PluginPage {
  view: string
  items: {
    folder: string
    data: unknown
    threads: { id: string; title: string; status: string; updated_at: string }[]
  }[]
}

export interface PluginInstallInput {
  repository: string
  ref?: string
  paths?: string[]
  plugin_id?: string
  source_id?: string
  auto_update?: boolean
}

export interface PluginInstallResult {
  plugins: Plugin[]
  skipped?: { name: string; reason: string }[]
}

export const pluginCatalogQuery = queryOptions({
  queryKey: keys.plugins,
  queryFn: async ({ client }) => {
    const data = await get<{ plugins?: Plugin[] | null; sources?: PluginSource[] | null }>('/v1/plugins')
    void client.invalidateQueries({ queryKey: keys.mcpApps, exact: true })
    return {
      plugins: data.plugins ?? [],
      sources: data.sources ?? [],
    } satisfies PluginCatalog
  },
})

export const pluginsQuery = queryOptions({
  ...pluginCatalogQuery,
  select: (data) => data.plugins,
})

export const pluginPageQuery = (plugin: string) => queryOptions({
  queryKey: keys.pluginPage(plugin),
  queryFn: () => get<PluginPage>(`/v1/plugins/${encodeURIComponent(plugin)}/page`),
  refetchInterval: 5000,
})

export function setPluginEnabled(id: string, enabled: boolean): Promise<Plugin> {
  return put<Plugin>(`/v1/plugins/${encodeURIComponent(id)}/enabled`, { enabled })
}

export function savePluginConfig(id: string, values: Record<string, string>): Promise<Plugin> {
  return put<Plugin>(`/v1/plugins/${encodeURIComponent(id)}/config`, { values })
}

export function connectPlugin(id: string): Promise<ConnectionStart> {
  return post<ConnectionStart>(`/v1/plugins/${encodeURIComponent(id)}/connect`)
}

export function disconnectPluginAccount(accountID: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/plugins/accounts/${encodeURIComponent(accountID)}`)
}

export function pluginQRStatus(sessionID: string): Promise<ConnectionQRStatus> {
  return get<ConnectionQRStatus>(`/v1/plugins/qr/${encodeURIComponent(sessionID)}/status`)
}

export function submitPluginQRPassword(sessionID: string, password: string): Promise<ConnectionQRStatus> {
  return post<ConnectionQRStatus>(`/v1/plugins/qr/${encodeURIComponent(sessionID)}/password`, { password })
}

export function closePluginQR(sessionID: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/plugins/qr/${encodeURIComponent(sessionID)}`)
}

export function installPlugin(input: PluginInstallInput): Promise<PluginInstallResult> {
  return post<PluginInstallResult>('/v1/plugins/install', input)
}

export function updatePlugin(id: string): Promise<PluginInstallResult> {
  return post<PluginInstallResult>(`/v1/plugins/${encodeURIComponent(id)}/update`)
}

export function removePlugin(id: string): Promise<void> {
  return del<void>(`/v1/plugins/${encodeURIComponent(id)}`)
}

export function addPluginSource(input: PluginInstallInput): Promise<PluginSource> {
  return post<PluginSource>('/v1/plugins/sources', input)
}

export function removePluginSource(id: string): Promise<void> {
  return del<void>(`/v1/plugins/sources/${encodeURIComponent(id)}`)
}

export function refreshPluginSources(): Promise<PluginCatalog> {
  return post<PluginCatalog>('/v1/plugins/sources/refresh')
}

export function openPluginView(session: string, input: { reference: string; file: string; name: string; title: string }): Promise<PluginView> {
  return post<PluginView>(`/v1/sessions/${encodeURIComponent(session)}/plugin-views`, input)
}

type ShownView = { reference: string; file: string }

const viewStatePath = (session: string, view: ShownView) =>
  `/v1/sessions/${session}/plugin-views/state?${new URLSearchParams(view)}`

export const pluginViewStateQuery = (session: string, view: ShownView) => queryOptions({
  queryKey: keys.pluginViewState(session, view.reference, view.file),
  queryFn: () => get<unknown>(viewStatePath(session, view)),
  staleTime: Infinity,
  gcTime: 0,
  retry: false,
})

export function savePluginViewState(session: string, view: ShownView, state: unknown): Promise<void> {
  return put<void>(viewStatePath(session, view), state ?? null)
}

export type { ConnectionQRStart, ConnectionQRStatus, ConnectionStart, IntegrationConnectionAccount }
