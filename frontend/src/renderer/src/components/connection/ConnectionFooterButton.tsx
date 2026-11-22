import { ChevronsUpDown } from 'lucide-react'
import { clientRuntime } from '@/lib/clientRuntime'
import { backendName, connectionStatusDisplay } from '@/lib/connectionDisplay'
import { BackendSwitcherMenu } from './BackendSwitcherMenu'
import { useBackendSwitcher } from './useBackendSwitcher'

// The sidebar's connection control: shows the active Cloud org/user identity or
// switches between Local and remote backends when multiple endpoints exist.
export function ConnectionFooterButton({ onOpenConnect }: { onOpenConnect: () => void }) {
  const switcher = useBackendSwitcher()
  const { status, url, mode, user, remotes, setOpen } = switcher
  const localEnabled = clientRuntime.capabilities.localBackend
  const isCloud = mode === 'cloud'

  if (!isCloud && (localEnabled ? 1 : 0) + remotes.length <= 1) return null

  const { dot } = connectionStatusDisplay(status)
  const displayName = isCloud && user?.org_name ? user.org_name : backendName(url)
  const subtitle = isCloud && user ? user.display_name || user.email : undefined

  return (
    <BackendSwitcherMenu
      switcher={switcher}
      placement="above"
      onConnectServer={onOpenConnect}
      trigger={
        <button
          type="button"
          onClick={() => setOpen((value) => !value)}
          title={subtitle ? `${displayName} · ${subtitle} (${url})` : url}
          className="group flex w-full items-center gap-2 rounded-lg px-2.5 py-1 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-list-hover"
        >
          <span className="grid size-[18px] shrink-0 place-items-center">
            <span className={`size-1.5 rounded-full ${dot}`} />
          </span>
          <span className="min-w-0 flex-1 text-left">
            <span className="flex items-center gap-1.5">
              <span className="min-w-0 truncate">{displayName}</span>
              <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-3">
                {isCloud ? 'Cloud' : 'Local'}
              </span>
            </span>
            {subtitle ? <span className="block truncate text-[11px] font-normal text-ink-3">{subtitle}</span> : null}
          </span>
          <ChevronsUpDown size={13} className="shrink-0 text-ink-3" />
        </button>
      }
    />
  )
}
