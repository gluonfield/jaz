import { queryOptions } from '@tanstack/react-query'
import { apiFetch, del, get, post } from '@/lib/api/client'
import { readAPIResponse } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'

// A plugin is a folder of skills and the views they show, installed on the Jaz server.
export interface Plugin {
  name: string
  display_name: string
  description: string
  version: string
  skills: string[]
  views: string[]
  mcp_servers: string[]
  icon_data_url?: string
  warnings?: string[]
  page?: { view: string }
  source?: { repository: string; ref?: string; commit: string }
}

export interface PluginPage {
  view: string
  items: {
    folder: string
    data: unknown
    threads: { id: string; title: string; status: string; updated_at: string }[]
  }[]
}

export type Installed = { plugins: Plugin[]; skipped?: { name: string; reason: string }[] }

export const pluginsQuery = queryOptions({
  queryKey: keys.plugins,
  queryFn: async () => (await get<Installed>('/v1/plugins')).plugins,
})

export const pluginPageQuery = (plugin: string) => queryOptions({
  queryKey: keys.pluginPage(plugin),
  queryFn: () => get<PluginPage>(`/v1/plugins/${encodeURIComponent(plugin)}/page`),
  refetchInterval: 5000,
})

// Installs every plugin in a zip: one plugin's folder, or a whole repository.
export async function installPlugins(archive: File): Promise<Plugin[]> {
  const response = await apiFetch('/v1/plugins', { method: 'POST', headers: { 'Content-Type': 'application/zip' }, body: archive })
  return (await readAPIResponse<Installed>(response)).plugins
}

export function removePlugin(name: string): Promise<void> {
  return del<void>(`/v1/plugins/${encodeURIComponent(name)}`)
}

export function installRepository(input: { repository: string; ref?: string }): Promise<Installed> {
  return post<Installed>('/v1/plugins/repository', input)
}

export function updatePlugin(name: string): Promise<Installed> {
  return post<Installed>(`/v1/plugins/${encodeURIComponent(name)}/update`)
}
