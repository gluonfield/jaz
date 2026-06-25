import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get } from './client'
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
