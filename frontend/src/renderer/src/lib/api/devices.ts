import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { apiBaseUrl, apiFetch, get, isLocalBackendUrl, normalizeBaseUrl } from './client'
import { readAPIResponse } from '@/lib/api/response'
import { clientRuntime } from '@/lib/clientRuntime'
import type { Device, DeviceConnectionLink, DeviceList, DevicePairing, PairingPoll } from './types'

export const devicesQuery = queryOptions({
  queryKey: keys.devices,
  queryFn: async () => {
    const result = await deviceRequest<DeviceList>('/v1/devices')
    if (result.can_manage && clientRuntime.getDeviceIdentity) {
      result.current_device_id = (await clientRuntime.getDeviceIdentity()).device_id
    }
    return result
  },
  refetchInterval: 5_000,
})

export const deviceConnectionLinkQuery = queryOptions({
  queryKey: keys.deviceConnectionLink,
  refetchOnWindowFocus: false,
  queryFn: () => deviceRequest<DeviceConnectionLink>('/v1/devices/connection-link'),
})

export function updateDeviceConnectionLink(publicURL: string): Promise<DeviceConnectionLink> {
  return deviceRequest<DeviceConnectionLink>('/v1/devices/connection-link', 'PUT', { public_url: publicURL })
}

export function approvePairing(id: string): Promise<{ pairing: DevicePairing }> {
  return deviceRequest<{ pairing: DevicePairing }>(`/v1/devices/pairing-requests/${encodeURIComponent(id)}/approve`, 'POST')
}

export function rejectPairing(id: string): Promise<{ pairing: DevicePairing }> {
  return deviceRequest<{ pairing: DevicePairing }>(`/v1/devices/pairing-requests/${encodeURIComponent(id)}/reject`, 'POST')
}

export function revokeDevice(id: string): Promise<{ device: Device }> {
  return deviceRequest<{ device: Device }>(`/v1/devices/${encodeURIComponent(id)}`, 'DELETE')
}

async function deviceRequest<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const url = apiBaseUrl()
  const headers = new Headers({ 'Content-Type': 'application/json' })
  if (isLocalBackendUrl(url) && clientRuntime.startLocalBackend) {
    const local = await clientRuntime.startLocalBackend()
    if (local.ok && local.key && normalizeBaseUrl(local.url ?? '') === url) {
      headers.set('Authorization', `Bearer ${local.key}`)
    }
  }
  return readAPIResponse<T>(await apiFetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) }, url))
}

export function pollPairing(id: string, secret: string): Promise<PairingPoll> {
  return get<PairingPoll>(`/v1/devices/pairing-requests/${encodeURIComponent(id)}`, { headers: { 'X-Jaz-Pairing-Secret': secret } })
}
