import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { del, get, post } from './client'
import type { ConnectionQRStatus, ConnectionStart, IntegrationPlugin } from './types'

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

export function startConnectionPlugin(id: string): Promise<ConnectionStart> {
  return post<ConnectionStart>(`/v1/connections/plugins/${encodeURIComponent(id)}/connect`)
}

export function connectionQRStatus(id: string): Promise<ConnectionQRStatus> {
  return get<ConnectionQRStatus>(`/v1/connections/qr/${encodeURIComponent(id)}`)
}

export function submitConnectionQRPassword(id: string, password: string): Promise<ConnectionQRStatus> {
  return post<ConnectionQRStatus>(`/v1/connections/qr/${encodeURIComponent(id)}/password`, { password })
}

export function closeConnectionQR(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/connections/qr/${encodeURIComponent(id)}`)
}

export function disconnectConnectionAccount(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/connections/accounts/${encodeURIComponent(id)}`)
}
