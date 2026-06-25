import { ChevronsUpDown, Plus } from 'lucide-react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { backendName, connectionStatusDisplay, describeBackend, localBackendLabel, sameBackend } from '@/lib/connectionDisplay'
import { useBackendSwitcher } from './useBackendSwitcher'

// The sidebar's connection control: a switcher popover for which backend Jaz is
// on, shown only once there's more than one backend to switch between. With just
// the local machine there's nothing to switch, so the footer stays clean and
// connecting a server lives in Settings instead.
export function ConnectionFooterButton({ onOpenConnect }: { onOpenConnect: () => void }) {
  const { open, setOpen, status, url, remotes, switchLocal, switchRemote } = useBackendSwitcher()

  if (remotes.length === 0) return null

  const { dot } = connectionStatusDisplay(status)

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="above"
      align="start"
      trigger={<Trigger url={url} name={backendName(url)} dot={dot} onClick={() => setOpen((value) => !value)} />}
    >
      <p className="px-2.5 pb-1 pt-1 text-[11px] font-medium text-ink-3">Run jaz on</p>
      <MenuRow selected={describeBackend(url).local} onClick={switchLocal}>
        {localBackendLabel()}
      </MenuRow>
      {remotes.map((remote) => (
        <MenuRow key={remote.url} selected={sameBackend(url, remote.url)} onClick={() => switchRemote(remote.url)}>
          {remote.label}
        </MenuRow>
      ))}
      <div className="my-1 h-px bg-border/70" />
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

function Trigger({ url, name, dot, onClick }: { url: string; name: string; dot: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={url}
      className="group flex w-full items-center gap-2 rounded-full px-2.5 py-1.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2"
    >
      <span className="min-w-0 flex-1 truncate text-left">{name}</span>
      <span className={`size-1.5 shrink-0 rounded-full ${dot}`} />
      <ChevronsUpDown size={13} className="shrink-0 text-ink-3" />
    </button>
  )
}
