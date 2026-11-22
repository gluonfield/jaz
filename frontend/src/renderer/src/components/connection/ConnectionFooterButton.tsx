import { ChevronsUpDown, Cloud, Laptop, LogIn, Plus } from 'lucide-react'
import { CloudIdentityBadge } from '@/components/connection/BackendSwitcher'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { clientRuntime } from '@/lib/clientRuntime'
import { backendName, connectionStatusDisplay, describeBackend, localBackendLabel, sameBackend } from '@/lib/connectionDisplay'
import { useBackendSwitcher } from './useBackendSwitcher'

// The sidebar's connection control: shows the active Cloud org/user identity or
// switches between Local and remote backends when multiple endpoints exist.
export function ConnectionFooterButton({ onOpenConnect }: { onOpenConnect: () => void }) {
  const {
    open,
    setOpen,
    status,
    url,
    mode,
    auth,
    oidcLoginUrl,
    user,
    remotes,
    switchLocal,
    switchRemote,
    signInWithSSO,
  } = useBackendSwitcher()
  const localEnabled = clientRuntime.capabilities.localBackend
  const isCloud = mode === 'cloud'

  if (!isCloud && (localEnabled ? 1 : 0) + remotes.length <= 1) return null

  const { dot } = connectionStatusDisplay(status)
  const displayName = isCloud && user?.org_name ? user.org_name : backendName(url)
  const subtitle = isCloud && user ? user.display_name || user.email : undefined

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="above"
      align="start"
      trigger={
        <Trigger
          url={url}
          name={displayName}
          subtitle={subtitle}
          isCloud={isCloud}
          dot={dot}
          onClick={() => setOpen((value) => !value)}
        />
      }
    >
      {isCloud && user ? (
        <>
          <CloudIdentityBadge user={user} />
          <div className="my-1.5 h-px bg-border/70" />
        </>
      ) : null}
      <p className="px-2.5 pb-1 pt-1 text-[11px] font-medium text-ink-3">
        {localEnabled ? 'Run jaz on' : 'Cloud workspaces'}
      </p>
      {localEnabled ? (
        <MenuRow selected={describeBackend(url).local} onClick={switchLocal}>
          <span className="flex items-center gap-2">
            <Laptop size={12} className="shrink-0 text-ink-3" />
            {localBackendLabel()}
          </span>
        </MenuRow>
      ) : null}
      {remotes.map((remote) => (
        <MenuRow key={remote.url} selected={sameBackend(url, remote.url)} onClick={() => switchRemote(remote.url)}>
          <span className="flex items-center gap-2">
            {remote.mode === 'cloud' || remote.auth === 'oidc' ? (
              <Cloud size={12} className="shrink-0 text-primary" />
            ) : null}
            <span className="truncate">{remote.label}</span>
          </span>
        </MenuRow>
      ))}
      <div className="my-1 h-px bg-border/70" />
      {isCloud && auth === 'oidc' ? (
        <MenuRow onClick={() => signInWithSSO(url, oidcLoginUrl)}>
          <span className="flex items-center gap-2">
            <LogIn size={13} />
            {user ? 'Re-authenticate SSO' : 'Sign in with SSO'}
          </span>
        </MenuRow>
      ) : null}
      <MenuRow
        onClick={() => {
          setOpen(false)
          onOpenConnect()
        }}
      >
        <span className="flex items-center gap-2">
          <Plus size={13} />
          Connect to a server
        </span>
      </MenuRow>
    </Popover>
  )
}

function Trigger({
  url,
  name,
  subtitle,
  isCloud,
  dot,
  onClick,
}: {
  url: string
  name: string
  subtitle?: string
  isCloud: boolean
  dot: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={subtitle ? `${name} · ${subtitle} (${url})` : url}
      className="group flex w-full items-center gap-2 rounded-lg px-2.5 py-1 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-list-hover"
    >
      <span className="grid size-[18px] shrink-0 place-items-center">
        <span className={`size-1.5 rounded-full ${dot}`} />
      </span>
      <span className="min-w-0 flex-1 text-left">
        <span className="flex items-center gap-1.5">
          <span className="min-w-0 truncate">{name}</span>
          <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-3">
            {isCloud ? 'Cloud' : 'Local'}
          </span>
        </span>
        {subtitle ? <span className="block truncate text-[11px] font-normal text-ink-3">{subtitle}</span> : null}
      </span>
      <ChevronsUpDown size={13} className="shrink-0 text-ink-3" />
    </button>
  )
}
