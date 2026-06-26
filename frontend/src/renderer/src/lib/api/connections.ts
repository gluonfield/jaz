import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { del, get, post } from './client'
import type { IntegrationPlugin } from './types'

export const connectionPluginsQuery = queryOptions({
  queryKey: keys.connectionPlugins,
  queryFn: async () => {
    const data = await get<{ plugins: IntegrationPlugin[] | null }>('/v1/connections/plugins')
    return data.plugins ?? []
  },
})

export function getConnectionPlugin(id: string): Promise<IntegrationPlugin> {
  return get<IntegrationPlugin>(`/v1/connections/plugins/${encodeURIComponent(id)}`)
}

export function startConnectionPlugin(id: string): Promise<{ auth_url: string }> {
  return post<{ auth_url: string }>(`/v1/connections/plugins/${encodeURIComponent(id)}/connect`)
}

export function disconnectConnectionAccount(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/connections/accounts/${encodeURIComponent(id)}`)
}
