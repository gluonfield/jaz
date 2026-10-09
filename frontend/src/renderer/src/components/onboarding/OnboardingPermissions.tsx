import { useMutation, useQuery } from '@tanstack/react-query'
import { Bell, CircleCheck, LoaderCircle, type LucideIcon, Mic, MousePointerClick, ScreenShare } from 'lucide-react'
import type { ComputerPermission } from '@shared/computerControl'
import type { MicrophoneAccess } from '@shared/microphone'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { keys } from '@/lib/query/keys'
import type { OnboardingStep } from './OnboardingParts'

// macOS gates computer use, voice and notifications behind privacy
// permissions; the desktop app asks for them during setup.
export const permissionsOnboarding = Boolean(window.jaz) && /Mac/i.test(navigator.platform)

type PermissionState = 'granted' | 'needed' | 'off' | 'waiting'

export type PermissionRow = {
  title: string
  detail: string
  icon: LucideIcon
  state: PermissionState
  allow: () => void
}

// The rows of the current permission slide, with live state: macOS changes
// these outside Jaz, so the statuses that can be read are polled.
export function usePermissionRows(step: OnboardingStep): PermissionRow[] {
  const jaz = window.jaz
  const toast = useToast()
  const onError = (error: Error) => toast(error.message, 'danger')
  const computer = useQuery({
    queryKey: keys.computerStatus,
    queryFn: () => jaz!.computer.status(),
    enabled: step === 'computer',
    refetchInterval: 1500,
  })
  const microphone = useQuery({
    queryKey: keys.microphoneAccess,
    queryFn: () => jaz!.microphone.status(),
    enabled: step === 'voice',
    refetchInterval: 1500,
  })
  const allowComputer = useMutation({ mutationFn: (permission: ComputerPermission) => jaz!.computer.allow(permission), onError })
  const allowMicrophone = useMutation({ mutationFn: () => jaz!.microphone.allow(), onSettled: () => microphone.refetch(), onError })
  const notifications = useMutation({ mutationFn: () => jaz!.notifications.test(), onError })
  if (step === 'computer') {
    const granted = computer.data?.permissions
    return [
      {
        title: 'Accessibility',
        detail: 'Click, type and use apps for you',
        icon: MousePointerClick,
        state: granted?.accessibility ? 'granted' : 'needed',
        allow: () => allowComputer.mutate('accessibility'),
      },
      {
        title: 'Screen Recording',
        detail: 'See the apps it works in',
        icon: ScreenShare,
        state: granted?.screenRecording ? 'granted' : 'needed',
        allow: () => allowComputer.mutate('screenRecording'),
      },
    ]
  }
  if (step === 'voice') {
    return [
      {
        title: 'Microphone',
        detail: 'Talk to jaz and dictate',
        icon: Mic,
        state: microphoneState(microphone.data),
        allow: () => allowMicrophone.mutate(),
      },
      {
        title: 'Notifications',
        detail: 'Know when a thread finishes',
        icon: Bell,
        state: notifications.isPending ? 'waiting' : notifications.data === true ? 'granted' : notifications.data === false ? 'off' : 'needed',
        allow: () => (notifications.data === false ? void jaz!.notifications.openSettings().catch(onError) : notifications.mutate()),
      },
    ]
  }
  return []
}

function microphoneState(access: MicrophoneAccess | undefined): PermissionState {
  if (access === 'granted') {
    return 'granted'
  }
  return access === 'denied' || access === 'restricted' ? 'off' : 'needed'
}

const DETAIL: Partial<Record<PermissionState, string>> = {
  off: 'Turned off in System Settings',
  waiting: 'Answer the macOS prompt',
}

export function PermissionList({ rows }: { rows: PermissionRow[] }) {
  return (
    <div>
      <div className="divide-y divide-border rounded-[14px] bg-surface">
        {rows.map((row) => (
          <div key={row.title} className="flex min-h-[60px] items-center gap-3 px-3.5 py-2.5">
            <row.icon size={18} className="shrink-0 text-ink-2" />
            <div className="min-w-0 flex-1">
              <p className="text-[13.5px] font-medium text-ink">{row.title}</p>
              <p className="text-[12px] text-ink-3">{DETAIL[row.state] ?? row.detail}</p>
            </div>
            {row.state === 'granted' ? (
              <CircleCheck size={20} aria-label="Allowed" className="shrink-0 text-ok" />
            ) : row.state === 'waiting' ? (
              <LoaderCircle size={16} aria-label="Waiting" className="shrink-0 animate-spin text-ink-3" />
            ) : (
              <Button variant="primary" onClick={row.allow}>Allow</Button>
            )}
          </div>
        ))}
      </div>
      <p className="mt-2.5 text-center text-[12px] text-ink-3">You can change these any time in Settings.</p>
    </div>
  )
}
