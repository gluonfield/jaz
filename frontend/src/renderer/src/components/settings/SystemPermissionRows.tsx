import { Bell, CircleCheck, LoaderCircle, type LucideIcon, Mic, MousePointerClick, ScreenShare } from 'lucide-react'
import { SYSTEM_PERMISSION_TITLES, type PermissionState, type SystemPermission } from '@shared/systemPermissions'
import { Button } from '@/components/ui/Button'
import { useSystemPermissions } from '@/lib/systemPermissions'

const ROWS: Record<SystemPermission, { detail: string; icon: LucideIcon; action: string }> = {
  accessibility: { detail: 'Enable Jaz in System Settings to use apps for you', icon: MousePointerClick, action: 'Open Settings' },
  screenRecording: { detail: 'Enable Jaz in System Settings; reopen Jaz if asked', icon: ScreenShare, action: 'Open Settings' },
  microphone: { detail: 'Talk to jaz and dictate', icon: Mic, action: 'Allow' },
  notifications: { detail: 'Send a test to check notification delivery', icon: Bell, action: 'Send test' },
}

const DETAIL: Partial<Record<PermissionState, string>> = {
  granted: 'Allowed',
  off: 'Turned off in System Settings',
  unavailable: 'Needs a signed Jaz build',
}

const NOTIFICATION_DETAIL: Partial<Record<PermissionState, string>> = {
  granted: 'Test notification sent',
  off: 'Delivery not confirmed. Check System Settings.',
}

export function SystemPermissionRows({ permissions }: { permissions: SystemPermission[] }) {
  const { status, error, checking, refresh, allow } = useSystemPermissions()
  return permissions.map((permission) => {
    const { detail, icon: Icon, action } = ROWS[permission]
    const state = error ? undefined : status?.[permission]
    const waiting = allow.isPending && allow.variables === permission
    const failed = allow.isError && allow.variables === permission && state !== 'granted'
    const notification = permission === 'notifications'
    const feedback = error ? 'Couldn’t check permissions. Try again.'
      : failed ? 'Couldn’t open this permission. Try again in System Settings.'
        : checking ? 'Checking permission…' : undefined
    const hint = notification ? NOTIFICATION_DETAIL[state ?? 'needed'] : undefined
    return (
      <div key={permission} className="flex min-h-[60px] items-center gap-3 px-3.5 py-2.5">
        <Icon size={18} className="shrink-0 text-ink-2" />
        <div className="min-w-0 flex-1">
          <p className="text-[13.5px] font-medium text-ink">{SYSTEM_PERMISSION_TITLES[permission]}</p>
          <p className="text-[12px] text-ink-3" role={failed || error ? 'alert' : 'status'}>{waiting ? 'Waiting for macOS' : feedback ?? hint ?? DETAIL[state ?? 'needed'] ?? detail}</p>
        </div>
        {error ? <Button onClick={() => void refresh()}>Try again</Button> : checking || waiting ? (
          <LoaderCircle size={16} aria-label={waiting ? 'Waiting' : 'Checking'} className="shrink-0 animate-spin text-ink-3" />
        ) : state === 'unavailable' ? null : state === 'granted' ? (
          <CircleCheck size={20} aria-label={notification ? 'Test sent' : 'Allowed'} className="shrink-0 text-ok" />
        ) : (
          <Button variant="primary" disabled={allow.isPending} onClick={() => allow.mutate(permission)}>{state === 'off' ? 'Open Settings' : action}</Button>
        )}
      </div>
    )
  })
}
