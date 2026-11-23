import { queryOptions } from '@tanstack/react-query'
import { get, put } from './client'
import type { MachineFolderGrant } from './serverTypes'

export interface MachineBridgeShareConfig {
  enabled: boolean
  display_name: string
  grants: MachineFolderGrant[]
  allow_exec: boolean
}

export interface MachineBridgeStatus {
  device_id: string
  connected: boolean
  cloud_url?: string
  last_connected_at?: string | null
  last_error?: string
  config: MachineBridgeShareConfig
}

export const machineBridgeQueryKey = ['cloud', 'machine-bridge'] as const

function normalizeBridgeStatus(raw: Partial<MachineBridgeStatus> | null | undefined): MachineBridgeStatus {
  const cfg = raw?.config
  return {
    device_id: typeof raw?.device_id === 'string' ? raw.device_id : '',
    connected: Boolean(raw?.connected),
    cloud_url: typeof raw?.cloud_url === 'string' ? raw.cloud_url : '',
    last_connected_at: typeof raw?.last_connected_at === 'string' ? raw.last_connected_at : null,
    last_error: typeof raw?.last_error === 'string' ? raw.last_error : '',
    config: {
      enabled: cfg?.enabled ?? true,
      display_name: typeof cfg?.display_name === 'string' ? cfg.display_name : '',
      grants: Array.isArray(cfg?.grants) ? cfg.grants : [],
      allow_exec: Boolean(cfg?.allow_exec),
    },
  }
}

export async function getMachineBridge(signal?: AbortSignal): Promise<MachineBridgeStatus> {
  return normalizeBridgeStatus(await get<MachineBridgeStatus>('/v1/cloud/machine-bridge', { signal }))
}

export async function updateMachineBridge(config: MachineBridgeShareConfig): Promise<MachineBridgeStatus> {
  return normalizeBridgeStatus(await put<MachineBridgeStatus>('/v1/cloud/machine-bridge', config))
}

export const machineBridgeQuery = queryOptions({
  queryKey: machineBridgeQueryKey,
  queryFn: ({ signal }) => getMachineBridge(signal),
  refetchInterval: 10_000,
  retry: false,
})
