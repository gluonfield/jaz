import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, MonitorSmartphone, Trash2, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { approvePairing, devicesQuery, rejectPairing, revokeDevice } from '@/lib/api/devices'
import type { Device, DevicePairing } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export function DevicesSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const devices = useQuery(devicesQuery)
  const invalidate = () => queryClient.invalidateQueries({ queryKey: keys.devices })

  const approve = useMutation({
    mutationFn: approvePairing,
    onSuccess: () => toast('Approved device'),
    onError: (error: Error) => toast(`Couldn't approve device: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })
  const reject = useMutation({
    mutationFn: rejectPairing,
    onSuccess: () => toast('Rejected device'),
    onError: (error: Error) => toast(`Couldn't reject device: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })
  const revoke = useMutation({
    mutationFn: revokeDevice,
    onSuccess: () => toast('Revoked device'),
    onError: (error: Error) => toast(`Couldn't revoke device: ${error.message}`, 'danger'),
    onSettled: invalidate,
  })

  const data = devices.data
  const pending = (data?.pairings ?? []).filter((pairing) => pairing.status === 'pending')
  const approved = (data?.devices ?? []).filter((device) => device.status === 'approved')
  const revoked = (data?.devices ?? []).filter((device) => device.status === 'revoked')
  const busy = approve.isPending || reject.isPending || revoke.isPending

  return (
    <section className="py-5">
      <div>
        <p className="text-sm font-medium text-ink">Devices</p>
        <p className="mt-0.5 text-[13px] text-ink-2">Desktop and mobile clients allowed to use this backend.</p>
      </div>

      <div className="mt-4">
        {devices.isPending ? (
          <SkeletonRows count={3} />
        ) : devices.isError ? (
          <p className="py-2 text-[13px] text-danger">{devices.error.message}</p>
        ) : (
          <div className="space-y-5">
            {pending.length > 0 ? (
              <DeviceGroup title="Pending approval">
                {pending.map((pairing) => (
                  <PendingRow
                    key={pairing.id}
                    pairing={pairing}
                    busy={busy}
                    onApprove={() => approve.mutate(pairing.id)}
                    onReject={() => reject.mutate(pairing.id)}
                  />
                ))}
              </DeviceGroup>
            ) : null}

            <DeviceGroup title="Connected devices">
              {approved.length === 0 ? (
                <p className="rounded-card bg-surface px-3 py-3 text-[13px] text-ink-3">No approved devices yet.</p>
              ) : (
                approved.map((device) => (
                  <DeviceRow
                    key={device.id}
                    device={device}
                    current={device.id === data?.current_device_id}
                    busy={busy}
                    onRevoke={() => revoke.mutate(device.id)}
                  />
                ))
              )}
            </DeviceGroup>

            {revoked.length > 0 ? (
              <DeviceGroup title="Revoked">
                {revoked.map((device) => (
                  <DeviceRow key={device.id} device={device} revoked />
                ))}
              </DeviceGroup>
            ) : null}
          </div>
        )}
      </div>
    </section>
  )
}

function DeviceGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <p className="mb-2 text-[12px] font-medium text-ink-2">{title}</p>
      <div className="flex flex-col gap-px overflow-hidden rounded-card bg-border/70">{children}</div>
    </div>
  )
}

function PendingRow({
  pairing,
  busy,
  onApprove,
  onReject,
}: {
  pairing: DevicePairing
  busy: boolean
  onApprove: () => void
  onReject: () => void
}) {
  return (
    <div className="grid grid-cols-1 gap-3 bg-surface px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <DeviceSummary device={pairing.device} detail={`Requested ${formatDate(pairing.created_at)}`} />
      <div className="flex items-center gap-1.5 md:justify-self-end">
        <Button size="sm" variant="primary" disabled={busy} onClick={onApprove}>
          <Check size={13} />
          Approve
        </Button>
        <Button size="sm" variant="danger" disabled={busy} onClick={onReject}>
          <X size={13} />
          Reject
        </Button>
      </div>
    </div>
  )
}

function DeviceRow({
  device,
  current = false,
  revoked = false,
  busy = false,
  onRevoke,
}: {
  device: Device
  current?: boolean
  revoked?: boolean
  busy?: boolean
  onRevoke?: () => void
}) {
  const detail = revoked
    ? `Revoked ${formatDate(device.revoked_at)}`
    : device.last_seen_at
      ? `Last seen ${formatDate(device.last_seen_at)}${device.last_seen_ip ? ` from ${device.last_seen_ip}` : ''}`
      : 'Not seen yet'

  return (
    <div className="grid grid-cols-1 gap-3 bg-surface px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <DeviceSummary device={device} detail={detail} current={current} />
      {!revoked ? (
        <div className="flex items-center gap-1.5 md:justify-self-end">
          <Button size="sm" variant="danger" disabled={busy || current} onClick={onRevoke}>
            <Trash2 size={13} />
            Revoke
          </Button>
        </div>
      ) : null}
    </div>
  )
}

function DeviceSummary({
  device,
  detail,
  current = false,
}: {
  device: Device
  detail: string
  current?: boolean
}) {
  const metadata = deviceMetadata(device)
  return (
    <div className="flex min-w-0 items-start gap-3">
      <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full bg-bg text-ink-3 ring-1 ring-border/70">
        <MonitorSmartphone size={15} />
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <p className="truncate text-[13px] font-medium text-ink">{device.name}</p>
          {current ? (
            <span className="rounded-full bg-primary-soft px-1.5 py-0.5 text-[10px] font-medium text-primary-strong">
              This device
            </span>
          ) : null}
        </div>
        {metadata ? <p className="mt-0.5 truncate text-[12px] text-ink-2">{metadata}</p> : null}
        <p className="mt-0.5 truncate text-[12px] text-ink-3">{detail}</p>
        <p className="mt-0.5 font-mono text-[11px] text-ink-3">{shortDeviceID(device.id)}</p>
      </div>
    </div>
  )
}

function deviceMetadata(device: Device): string {
  return uniqueParts([
    device.platform,
    device.device_family,
    device.model_identifier,
    device.app_version ? `Jaz ${device.app_version}` : '',
  ]).join(' / ')
}

function uniqueParts(parts: Array<string | undefined>): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of parts) {
    const part = raw?.trim()
    if (!part) continue
    const key = part.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(part)
  }
  return out
}

function shortDeviceID(id: string): string {
  return id.length > 16 ? `${id.slice(0, 12)}...${id.slice(-4)}` : id
}

function formatDate(value?: string): string {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}
