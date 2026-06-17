import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { del, get, post } from './client'
import type { Device, DeviceConnectionLink, DeviceList, DevicePairing, PairingPoll } from './types'

export const devicesQuery = queryOptions({
  queryKey: keys.devices,
  queryFn: () => get<DeviceList>('/v1/devices'),
  refetchInterval: 5_000,
})

export const deviceConnectionLinkQuery = queryOptions({
  queryKey: keys.deviceConnectionLink,
  queryFn: () => get<DeviceConnectionLink>('/v1/devices/connection-link'),
})

export function approvePairing(id: string): Promise<{ pairing: DevicePairing }> {
  return post<{ pairing: DevicePairing }>(`/v1/devices/pairing-requests/${encodeURIComponent(id)}/approve`)
}

export function rejectPairing(id: string): Promise<{ pairing: DevicePairing }> {
  return post<{ pairing: DevicePairing }>(`/v1/devices/pairing-requests/${encodeURIComponent(id)}/reject`)
}

export function revokeDevice(id: string): Promise<{ device: Device }> {
  return del<{ device: Device }>(`/v1/devices/${encodeURIComponent(id)}`)
}

export function pollPairing(id: string, secret: string): Promise<PairingPoll> {
  const params = new URLSearchParams({ secret })
  return get<PairingPoll>(`/v1/devices/pairing-requests/${encodeURIComponent(id)}?${params}`)
}
