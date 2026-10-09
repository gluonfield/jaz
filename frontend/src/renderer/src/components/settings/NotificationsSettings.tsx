import { useMutation } from '@tanstack/react-query'
import { Button } from '@/components/ui/Button'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { useThreadNotificationsEnabled } from '@/lib/notificationSettings'

export function NotificationsSettings() {
  const host = window.jaz?.notifications
  const [threadFinished, setThreadFinished] = useThreadNotificationsEnabled()
  const toast = useToast()
  const test = useMutation({
    mutationFn: () => host!.test(),
    onError: (error: Error) => toast(error.message, 'danger'),
  })
  const openSettings = useMutation({
    mutationFn: () => host!.openSettings(),
    onError: (error: Error) => toast(error.message, 'danger'),
  })
  return (
    <section className="py-4">
      <h1 className="text-lg font-semibold text-ink">Notifications</h1>
      <p className="mt-0.5 text-[13px] text-ink-2">Choose when Jaz can get your attention.</p>
      {!host ? <p className="mt-4 text-sm text-ink-2">Open Jaz desktop to get notifications.</p> : (
        <SettingsCard className="mt-4 divide-y divide-border overflow-hidden">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2.5">
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-medium text-ink">System notifications</p>
              <p className={`mt-0.5 text-[12px] ${test.data === false ? 'text-danger' : 'text-ink-3'}`}>
                {test.data === undefined && 'Send a test to check that Jaz can reach you.'}
                {test.data === true && 'Jaz can show notifications.'}
                {test.data === false && "macOS is blocking Jaz's notifications. Allow them in Settings."}
              </p>
            </div>
            <div className="flex gap-1">
              <Button disabled={test.isPending} onClick={() => test.mutate()}>Send test</Button>
              <Button disabled={openSettings.isPending} onClick={() => openSettings.mutate()}>Open Settings</Button>
            </div>
          </div>
          <div className="flex items-center gap-4 px-3 py-2.5">
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-medium text-ink">Thread finished</p>
              <p className="mt-0.5 text-[12px] text-ink-3">Notify when a thread finishes and Jaz isn't focused.</p>
            </div>
            <Switch checked={threadFinished} onChange={setThreadFinished} aria-label="Notify when a thread finishes" />
          </div>
        </SettingsCard>
      )}
    </section>
  )
}
