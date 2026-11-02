import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { MessageSquareCheck } from 'lucide-react'
import type { NotchAccess, NotificationMode } from '@shared/notifications'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { SystemPermissionRows } from '@/components/settings/SystemPermissionRows'
import { clientRuntime } from '@/lib/clientRuntime'
import { useNotificationMode } from '@/lib/notificationSettings'
import { keys } from '@/lib/query/keys'
import { hasSystemPermissions } from '@/lib/systemPermissions'

const MODES: { value: NotificationMode; label: string; description: string }[] = [
  { value: 'off', label: 'Off', description: 'Never notify' },
  { value: 'center', label: 'Notification Center', description: 'A standard macOS notification' },
  { value: 'notch', label: 'Notch', description: 'Who, what and the outcome beside the camera' },
  { value: 'notch_ears', label: 'Notch, ears only', description: 'Only who and what, for shared screens' },
]

const NOTCH: Record<NotchAccess, string> = {
  ready: 'Shown beside the notch only while no Focus is on; otherwise Notification Center.',
  notifications: 'Allow notifications above to use the notch.',
  ask: 'Share your Focus status so Jaz uses the notch only while no Focus is on.',
  denied: 'Focus status isn’t shared, so Jaz uses Notification Center. Allow Jaz in System Settings → Privacy & Security → Focus.',
  unavailable: 'Jaz can’t read your Focus status, so it uses Notification Center.',
}

export function NotificationsSettings() {
  const [mode, setMode] = useNotificationMode()
  const notch = mode === 'notch' || mode === 'notch_ears'
  const queryClient = useQueryClient()
  const access = useQuery({ queryKey: keys.notchAccess, queryFn: () => clientRuntime.notchAccess!(), enabled: notch && hasSystemPermissions })
  const share = useMutation({
    mutationFn: () => clientRuntime.shareFocusStatus!(),
    onSuccess: (value) => queryClient.setQueryData(keys.notchAccess, value),
  })
  const choose = (value: string) => {
    const next = value as NotificationMode
    setMode(next)
    if (next === 'notch' || next === 'notch_ears') share.mutate()
  }
  return (
    <section className="py-4">
      <h1 className="text-lg font-semibold text-ink">Notifications</h1>
      <p className="mt-0.5 text-[13px] text-ink-2">Choose when Jaz can get your attention.</p>
      {!window.jaz ? <p className="mt-4 text-sm text-ink-2">Open Jaz desktop to get notifications.</p> : (
        <SettingsCard className="mt-4 divide-y divide-border">
          {hasSystemPermissions && <SystemPermissionRows permissions={['notifications']} />}
          <div className="flex min-h-[60px] items-center gap-3 px-3.5 py-2.5">
            <MessageSquareCheck size={18} className="shrink-0 text-ink-2" />
            <div className="min-w-0 flex-1">
              <p className="text-[13.5px] font-medium text-ink">Done, replies and questions</p>
              <p className="text-[12px] text-ink-3">{notch ? NOTCH[access.data ?? 'ready'] : "When Jaz isn't focused."}</p>
            </div>
            {notch && access.data === 'ask' && (
              <Button disabled={share.isPending} onClick={() => share.mutate()}>Share Focus</Button>
            )}
            <Select
              aria-label="How Jaz notifies"
              value={mode}
              options={hasSystemPermissions ? MODES : MODES.slice(0, 2)}
              onChange={choose}
            />
          </div>
        </SettingsCard>
      )}
    </section>
  )
}
