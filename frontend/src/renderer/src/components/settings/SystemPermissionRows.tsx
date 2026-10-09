import { Bell, CircleCheck, LoaderCircle, type LucideIcon, Mic, MousePointerClick, ScreenShare } from 'lucide-react'
import { SYSTEM_PERMISSION_TITLES, type PermissionState, type SystemPermission } from '@shared/systemPermissions'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { useSystemPermissions } from '@/lib/systemPermissions'

const ROWS: Record<SystemPermission, { detail: string; icon: LucideIcon }> = {
  accessibility: { detail: 'Click, type and use apps for you', icon: MousePointerClick },
  screenRecording: { detail: 'See the apps it works in', icon: ScreenShare },
  microphone: { detail: 'Talk to jaz and dictate', icon: Mic },
  notifications: { detail: 'Banners and sounds when work is done', icon: Bell },
}

const DETAIL: Partial<Record<PermissionState, string>> = {
  off: 'Turned off in System Settings',
  unavailable: 'Needs a signed Jaz build',
}

// One row per permission: Allow, a check once granted, or the way back from a
// refusal. Callers supply the card and its dividers.
export function SystemPermissionRows({ permissions }: { permissions: SystemPermission[] }) {
  const { status, allow } = useSystemPermissions()
  const toast = useToast()
  return permissions.map((permission) => {
    const { detail, icon: Icon } = ROWS[permission]
    const state = status?.[permission] ?? 'needed'
    const waiting = allow.isPending && allow.variables === permission
    return (
      <div key={permission} className="flex min-h-[60px] items-center gap-3 px-3.5 py-2.5">
        <Icon size={18} className="shrink-0 text-ink-2" />
        <div className="min-w-0 flex-1">
          <p className="text-[13.5px] font-medium text-ink">{SYSTEM_PERMISSION_TITLES[permission]}</p>
          <p className="text-[12px] text-ink-3">{waiting ? 'Waiting for macOS' : DETAIL[state] ?? detail}</p>
        </div>
        {state === 'unavailable' ? null : state === 'granted' ? (
          <CircleCheck size={20} aria-label="Allowed" className="shrink-0 text-ok" />
        ) : waiting ? (
          <LoaderCircle size={16} aria-label="Waiting" className="shrink-0 animate-spin text-ink-3" />
        ) : (
          <Button variant="primary" onClick={() => allow.mutate(permission, { onError: (error) => toast(error.message, 'danger') })}>Allow</Button>
        )}
      </div>
    )
  })
}
