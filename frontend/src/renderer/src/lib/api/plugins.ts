import { queryOptions } from '@tanstack/react-query'
import { apiFetch, del, get } from '@/lib/api/client'
import { readAPIResponse } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'

// A plugin is a folder of skills and the views they show, installed on the Jaz server.
export interface Plugin {
  name: string
  title: string
  description: string
  version: string
  skills: string[]
  views: string[]
}

type Installed = { plugins: Plugin[] }

export const pluginsQuery = queryOptions({
  queryKey: keys.plugins,
  queryFn: async () => (await get<Installed>('/v1/plugins')).plugins,
})

// Installs every plugin in a zip: one plugin's folder, or a whole repository.
export async function installPlugins(archive: File): Promise<Plugin[]> {
  const response = await apiFetch('/v1/plugins', { method: 'POST', headers: { 'Content-Type': 'application/zip' }, body: archive })
  return (await readAPIResponse<Installed>(response)).plugins
}

export function removePlugin(name: string): Promise<void> {
  return del<void>(`/v1/plugins/${encodeURIComponent(name)}`)
}
