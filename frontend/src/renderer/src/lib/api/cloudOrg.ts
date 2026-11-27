import { queryOptions } from '@tanstack/react-query'
import { keys } from '@/lib/query/keys'
import { del, get, patch, post, put } from './client'
import type { Plugin, PluginInstallInput, PluginSource } from './plugins'
import type { OrgIcon } from './serverTypes'

export const MAX_ORG_ICON_BYTES = 256 * 1024

export interface OrgIdentity {
  org_id: string
  org_name?: string
  org_icon?: OrgIcon
}

// The signed-in member's live org name and icon. Desktop serves the active Cloud
// workspace's identity on the same path.
export const orgIdentityQuery = queryOptions({
  queryKey: keys.cloudOrgIdentity,
  queryFn: ({ signal }) => get<OrgIdentity>('/v1/auth/me', { signal }),
  staleTime: 60_000,
})

export interface AdminOrg {
  id: string
  slug: string
  name: string
  icon: OrgIcon
}

export type OrgIconChange =
  | { kind: 'none' }
  | { kind: 'preset'; preset: string }
  | { kind: 'image'; data_base64: string }

export const adminOrgQuery = queryOptions({
  queryKey: keys.cloudAdminOrg,
  queryFn: ({ signal }) => get<AdminOrg>('/v1/admin/org', { signal }),
})

export function updateAdminOrg(change: { name?: string; icon?: OrgIconChange }): Promise<AdminOrg> {
  return patch<AdminOrg>('/v1/admin/org', change)
}

export interface OrgAPIKey {
  key_id: string
  kind: 'llm_provider' | 'integration'
  display_name: string
  configured: boolean
  label?: string
  last4?: string
  updated_at?: string
  updated_by?: { user_id: string; email?: string; display_name?: string }
}

export const INTEGRATION_KEY_PREFIX = 'integration-'

export const adminAPIKeysQuery = queryOptions({
  queryKey: keys.cloudAdminAPIKeys,
  queryFn: async ({ signal }) => (await get<{ keys: OrgAPIKey[] }>('/v1/admin/api-keys', { signal })).keys,
})

const apiKeyPath = (keyID: string) => `/v1/admin/api-keys/${encodeURIComponent(keyID)}`

export function saveOrgAPIKey(keyID: string, input: { api_key: string; label: string }): Promise<OrgAPIKey> {
  return put<OrgAPIKey>(apiKeyPath(keyID), input)
}

export function deleteOrgAPIKey(keyID: string): Promise<void> {
  return del<void>(apiKeyPath(keyID))
}

export type InstallationPolicy = 'INSTALLED_BY_DEFAULT' | 'AVAILABLE' | 'DISABLED'

export interface AdminPlugin extends Plugin {
  source_id: string
  // Empty while the plugin is only installable from the official marketplace.
  installation_policy: InstallationPolicy | ''
  installable: boolean
  marketplace_id?: string
}

// Removing or disabling a plugin interrupts the turns that were running with it.
export interface PluginRemoval {
  removed_plugin_ids: string[]
  fenced_turns: number
}

export const adminMarketplacesQuery = queryOptions({
  queryKey: keys.cloudAdminMarketplaces,
  queryFn: async ({ signal }) =>
    (await get<{ marketplaces: PluginSource[] }>('/v1/admin/marketplaces', { signal })).marketplaces,
})

export function addOrgMarketplace(input: PluginInstallInput): Promise<{ marketplace: PluginSource }> {
  return post<{ marketplace: PluginSource }>('/v1/admin/marketplaces', input)
}

export function removeOrgMarketplace(id: string): Promise<PluginRemoval> {
  return del<PluginRemoval>(`/v1/admin/marketplaces/${encodeURIComponent(id)}`)
}

export const adminPluginsQuery = queryOptions({
  queryKey: keys.cloudAdminPlugins,
  queryFn: async ({ signal }) => (await get<{ plugins: AdminPlugin[] }>('/v1/admin/plugins', { signal })).plugins,
})

const adminPluginPath = (id: string) => `/v1/admin/plugins/${encodeURIComponent(id)}`

// Installs one official-marketplace plugin ({ source_id, plugin_id }) or the plugins of a Git repository.
export function installOrgPlugin(
  input: PluginInstallInput | { source_id: string; plugin_id: string },
): Promise<{ plugins: AdminPlugin[] }> {
  return post<{ plugins: AdminPlugin[] }>('/v1/admin/plugins', input)
}

export function setOrgPluginPolicy(
  id: string,
  policy: InstallationPolicy,
): Promise<{ plugins: AdminPlugin[]; fenced_turns: number }> {
  return put<{ plugins: AdminPlugin[]; fenced_turns: number }>(`${adminPluginPath(id)}/policy`, {
    installation_policy: policy,
  })
}

export function uninstallOrgPlugin(id: string): Promise<PluginRemoval> {
  return del<PluginRemoval>(adminPluginPath(id))
}
