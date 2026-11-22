import { ChevronsUpDown, Cloud, Laptop } from 'lucide-react'
import { backendName, connectionStatusDisplay } from '@/lib/connectionDisplay'
import { BackendSwitcherMenu } from './BackendSwitcherMenu'
import { useBackendSwitcher } from './useBackendSwitcher'

export { CloudIdentityBadge } from './BackendSwitcherMenu'

export function BackendSwitcher({
  onConnectServer,
  onOpenCloudSettings,
}: {
  onConnectServer: () => void
  onOpenCloudSettings?: () => void
}) {
  const switcher = useBackendSwitcher()
  const { status, url, mode, user, cloud, setOpen } = switcher
  const { dot } = connectionStatusDisplay(status)
  const isPrimaryCloud = mode === 'cloud'
  const cloudAttached = !isPrimaryCloud && cloud.active

  return (
    <BackendSwitcherMenu
      switcher={switcher}
      placement="below"
      editable
      showRemember
      onConnectServer={onConnectServer}
      onOpenCloudSettings={onOpenCloudSettings}
      trigger={
        <button
          type="button"
          onClick={() => setOpen((value) => !value)}
          className="flex w-full items-center gap-2 rounded-card bg-list-hover px-3 py-2 text-left transition-colors duration-150 hover:bg-list-active"
        >
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wide text-ink-3">
              {isPrimaryCloud ? (
                <>
                  <Cloud size={10} className="text-primary" />
                  Cloud
                </>
              ) : cloudAttached ? (
                <>
                  <Laptop size={10} />
                  Local
                  <span aria-hidden>·</span>
                  <Cloud size={10} className="text-primary" />
                  Cloud
                </>
              ) : (
                <>
                  <Laptop size={10} />
                  Local
                </>
              )}
            </span>
            <span className="block truncate text-[13px] font-medium text-ink">
              {isPrimaryCloud && user?.org_name ? user.org_name : backendName(url)}
            </span>
            {isPrimaryCloud && user ? (
              <span className="block truncate text-[11px] text-ink-3">{user.email || user.display_name}</span>
            ) : cloudAttached && cloud.user ? (
              <span className="block truncate text-[11px] text-ink-3">
                {cloud.user.org_name || cloud.user.email || cloud.user.display_name}
              </span>
            ) : null}
          </span>
          <span className={`size-1.5 shrink-0 rounded-full ${dot}`} />
          <ChevronsUpDown size={14} className="shrink-0 text-ink-3" />
        </button>
      }
    />
  )
}
