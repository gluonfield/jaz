import { MessageSquareCheck } from 'lucide-react'
import { Switch } from '@/components/ui/Switch'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { SystemPermissionRows } from '@/components/settings/SystemPermissionRows'
import { useThreadNotificationsEnabled } from '@/lib/notificationSettings'
import { hasSystemPermissions } from '@/lib/systemPermissions'

export function NotificationsSettings() {
  const [threadFinished, setThreadFinished] = useThreadNotificationsEnabled()
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
              <p className="text-[13.5px] font-medium text-ink">Thread finished</p>
              <p className="text-[12px] text-ink-3">Notify when a thread finishes and Jaz isn't focused.</p>
            </div>
            <Switch checked={threadFinished} onChange={setThreadFinished} aria-label="Notify when a thread finishes" />
          </div>
        </SettingsCard>
      )}
    </section>
  )
}
