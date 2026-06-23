import { ChevronsUpDown, MonitorSmartphone, Plus, Server } from 'lucide-react'
import { useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { useKnownBackends } from '@/lib/backends'
import { connectRemote, startLocal, useConnection } from '@/lib/connection'
import { backendName, connectionStatusDisplay, describeBackend, localBackendLabel, sameBackend } from '@/lib/connectionDisplay'

// The sidebar's connection control: shows which backend Jaz is on with a health
// dot. With no saved remotes there's nothing to switch between, so it just opens
// the connect screen to add one. Once a remote exists it becomes a switcher
// popover; "Connect to a server" there opens the full connect screen.
export function ConnectionFooterButton({ onOpenConnect }: { onOpenConnect: () => void }) {
  const { status, url } = useConnection()
  const remotes = useKnownBackends()
  const toast = useToast()
  const [open, setOpen] = useState(false)

  const backend = describeBackend(url)
  const name = backendName(url)
  const { dot } = connectionStatusDisplay(status)

  if (remotes.length === 0) {
    return <Trigger backend={backend} name={name} dot={dot} onClick={onOpenConnect} />
  }

  const switchTo = async (action: () => Promise<string | null>) => {
    setOpen(false)
    const err = await action()
    if (err) toast(err, 'danger')
  }

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="above"
      align="start"
      trigger={<Trigger backend={backend} name={name} dot={dot} switcher onClick={() => setOpen((value) => !value)} />}
    >
      <p className="px-2.5 pb-1 pt-1 text-[11px] font-medium text-ink-3">Run jaz on</p>
      <MenuRow selected={backend.local} onClick={() => switchTo(startLocal)}>
        {localBackendLabel()}
      </MenuRow>
      {remotes.map((remote) => (
        <MenuRow key={remote.url} selected={sameBackend(url, remote.url)} onClick={() => switchTo(() => connectRemote(remote.url))}>
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

function Trigger({
  backend,
  name,
  dot,
  switcher = false,
  onClick,
}: {
  backend: { local: boolean; title: string; url: string }
  name: string
  dot: string
  switcher?: boolean
  onClick: () => void
}) {
  const Icon = backend.local ? MonitorSmartphone : Server
  return (
    <button
      type="button"
      onClick={onClick}
      title={backend.url}
      className="group flex w-full items-center gap-2 rounded-full px-2.5 py-1.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2"
    >
      <Icon size={15} className="shrink-0 text-ink-2" />
      <span className="min-w-0 flex-1 truncate text-left">{name}</span>
      <span className={`size-1.5 shrink-0 rounded-full ${dot}`} />
      {switcher ? <ChevronsUpDown size={13} className="shrink-0 text-ink-3" /> : null}
    </button>
  )
}
