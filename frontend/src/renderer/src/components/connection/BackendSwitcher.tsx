import { Check, ChevronsUpDown, MonitorSmartphone, Pencil, Plus, Server, X } from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import { Input } from '@/components/ui/Input'
import { Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { type KnownBackend, renameBackend, useKnownBackends } from '@/lib/backends'
import { connectRemote, forgetBackend, startLocal, useConnection } from '@/lib/connection'
import {
  backendName,
  connectionStatusDisplay,
  describeBackend,
  localBackendLabel,
  sameBackend,
} from '@/lib/connectionDisplay'

// The backend selector that sits above the settings list: switch which machine
// Jaz runs on (everything below is configured for it), rename or forget saved
// servers, or connect a new one. Backends are the context for settings, not a
// settings page of their own.
export function BackendSwitcher({ onConnectServer }: { onConnectServer: () => void }) {
  const { status, url } = useConnection()
  const remotes = useKnownBackends()
  const toast = useToast()
  const [open, setOpen] = useState(false)

  const backend = describeBackend(url)
  const { dot } = connectionStatusDisplay(status)
  const Icon = backend.local ? MonitorSmartphone : Server

  const switchTo = async (action: () => Promise<string | null>) => {
    setOpen(false)
    const err = await action()
    if (err) toast(err, 'danger')
  }

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="below"
      align="start"
      trigger={
        <button
          type="button"
          onClick={() => setOpen((value) => !value)}
          className="flex w-full items-center gap-2 rounded-card bg-bg px-2.5 py-2 text-left ring-1 ring-border/70 transition-colors duration-150 hover:bg-surface-2"
        >
          <span className="grid size-7 shrink-0 place-items-center rounded-full bg-surface text-ink-2 ring-1 ring-border/70">
            <Icon size={14} />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-[10px] font-medium uppercase tracking-wide text-ink-3">Backend</span>
            <span className="block truncate text-[13px] font-medium text-ink">{backendName(url)}</span>
          </span>
          <span className={`size-1.5 shrink-0 rounded-full ${dot}`} />
          <ChevronsUpDown size={14} className="shrink-0 text-ink-3" />
        </button>
      }
    >
      <p className="px-2.5 pb-1 pt-1 text-[11px] font-medium text-ink-3">Run jaz on</p>
      <SwitchRow
        icon={<MonitorSmartphone size={14} />}
        label={localBackendLabel()}
        current={backend.local}
        onSwitch={() => switchTo(startLocal)}
      />
      {remotes.map((remote) => (
        <RemoteRow
          key={remote.url}
          backend={remote}
          current={sameBackend(url, remote.url)}
          onSwitch={() => switchTo(() => connectRemote(remote.url))}
          onRename={(label) => renameBackend(remote.url, label)}
          onForget={() => forgetBackend(remote.url)}
        />
      ))}
      <div className="my-1 h-px bg-border/70" />
      <button
        type="button"
        onClick={() => {
          setOpen(false)
          onConnectServer()
        }}
        className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      >
        <Plus size={13} />
        Connect to a server
      </button>
    </Popover>
  )
}

function SwitchRow({
  icon,
  label,
  current,
  onSwitch,
}: {
  icon: ReactNode
  label: string
  current: boolean
  onSwitch: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSwitch}
      className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
    >
      <span className="shrink-0 text-ink-3">{icon}</span>
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {current ? <Check size={13} className="shrink-0 text-primary" /> : null}
    </button>
  )
}

function RemoteRow({
  backend,
  current,
  onSwitch,
  onRename,
  onForget,
}: {
  backend: KnownBackend
  current: boolean
  onSwitch: () => void
  onRename: (label: string) => void
  onForget: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(backend.label)
  useEffect(() => setDraft(backend.label), [backend.label])

  const save = () => {
    setEditing(false)
    const next = draft.trim()
    if (next && next !== backend.label) onRename(next)
  }

  if (editing) {
    return (
      <div className="px-1 py-0.5">
        <Input
          autoFocus
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={save}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              save()
            } else if (event.key === 'Escape') {
              event.stopPropagation()
              setDraft(backend.label)
              setEditing(false)
            }
          }}
          aria-label="Backend name"
          className="h-7 text-[13px]"
        />
      </div>
    )
  }

  return (
    <div className="group/row flex h-7 items-center rounded-full pr-1 transition-colors duration-150 hover:bg-surface-2">
      <button
        type="button"
        onClick={onSwitch}
        className="flex min-w-0 flex-1 items-center gap-2 px-2.5 text-left text-[13px] text-ink"
      >
        <Server size={14} className="shrink-0 text-ink-3" />
        <span className="min-w-0 flex-1 truncate">{backend.label}</span>
        {current ? <Check size={13} className="shrink-0 text-primary" /> : null}
      </button>
      <RowAction icon={<Pencil size={11} />} label={`Rename ${backend.label}`} onClick={() => setEditing(true)} />
      {!current ? <RowAction icon={<X size={11} />} label={`Forget ${backend.label}`} onClick={onForget} /> : null}
    </div>
  )
}

function RowAction({ icon, label, onClick }: { icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className="grid size-5 shrink-0 place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface hover:text-ink focus-visible:opacity-100 group-hover/row:opacity-100"
    >
      {icon}
    </button>
  )
}
