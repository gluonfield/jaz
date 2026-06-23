import { LoaderCircle, MonitorSmartphone, Pencil, Plus, Server, Trash2 } from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import { RemoteServerForm } from '@/components/connection/RemoteServerForm'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { normalizeBaseUrl } from '@/lib/api/client'
import { renameBackend, useKnownBackends } from '@/lib/backends'
import { connectRemote, forgetBackend, startLocal, useConnection, type ConnectionStatus } from '@/lib/connection'
import { connectionStatusDisplay, describeBackend, localBackendLabel, sameBackend } from '@/lib/connectionDisplay'
import { relativeTime } from '@/lib/format/time'

export function BackendsSettings() {
  const { status, url } = useConnection()
  const remotes = useKnownBackends()
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const [addUrl, setAddUrl] = useState('')

  const localCurrent = describeBackend(url).local

  const run = async (target: string, action: () => Promise<string | null>): Promise<string | null> => {
    setBusy(target)
    setError(null)
    const err = await action()
    setBusy(null)
    if (err) setError(err)
    return err
  }

  const onAdd = async () => {
    const err = await run(normalizeBaseUrl(addUrl) || 'add', () => connectRemote(addUrl))
    if (!err) {
      setAdding(false)
      setAddUrl('')
    }
  }

  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">Backends</p>
          <p className="mt-0.5 text-[13px] text-ink-2">Machines Jaz can run on — this computer or any server you connect. Switch any time; the names are yours.</p>
        </div>
        {!adding ? (
          <Button variant="secondary" size="md" onClick={() => { setAdding(true); setError(null) }}>
            <Plus size={14} />
            Add a server
          </Button>
        ) : null}
      </div>

      <div className="mt-4 flex flex-col gap-px overflow-hidden rounded-card bg-border/70">
        <BackendRow
          icon={<MonitorSmartphone size={15} />}
          name={localBackendLabel()}
          detail="Runs the backend on this computer"
          current={localCurrent}
          status={status}
          busy={busy === 'local'}
          disabled={busy !== null}
          onSwitch={localCurrent ? undefined : () => run('local', startLocal)}
        />
        {remotes.map((backend) => {
          const current = sameBackend(url, backend.url)
          const seen = relativeTime(backend.lastConnectedAt)
          return (
            <BackendRow
              key={backend.url}
              icon={<Server size={15} />}
              name={backend.label}
              url={backend.url}
              detail={seen ? `Last connected ${seen}` : 'Not connected yet'}
              current={current}
              status={status}
              busy={busy === backend.url}
              disabled={busy !== null}
              onSwitch={current ? undefined : () => run(backend.url, () => connectRemote(backend.url))}
              onRename={(name) => renameBackend(backend.url, name)}
              onForget={current ? undefined : () => forgetBackend(backend.url)}
            />
          )
        })}
      </div>

      {error ? <p className="mt-3 rounded-card bg-danger-soft px-3 py-2 text-[12px] text-danger">{error}</p> : null}

      {adding ? (
        <div className="mt-3 rounded-card bg-surface p-3">
          <RemoteServerForm
            value={addUrl}
            onChange={setAddUrl}
            onSubmit={onAdd}
            onBack={() => {
              setAdding(false)
              setError(null)
            }}
            busy={busy !== null}
          />
        </div>
      ) : null}
    </section>
  )
}

function BackendRow({
  icon,
  name,
  url,
  detail,
  current,
  status,
  busy,
  disabled,
  onSwitch,
  onRename,
  onForget,
}: {
  icon: ReactNode
  name: string
  url?: string
  detail: string
  current: boolean
  status: ConnectionStatus
  busy: boolean
  disabled: boolean
  onSwitch?: () => void
  onRename?: (name: string) => void
  onForget?: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(name)
  useEffect(() => setDraft(name), [name])

  const saveName = () => {
    setEditing(false)
    const next = draft.trim()
    if (next && next !== name) onRename?.(next)
  }

  return (
    <div className="grid grid-cols-1 gap-3 bg-surface px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <div className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-full bg-bg text-ink-3 ring-1 ring-border/70">
          {icon}
        </div>
        <div className="min-w-0">
          {editing ? (
            <Input
              autoFocus
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onBlur={saveName}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  saveName()
                } else if (event.key === 'Escape') {
                  setDraft(name)
                  setEditing(false)
                }
              }}
              aria-label="Server name"
              className="h-7 max-w-[240px] text-[13px]"
            />
          ) : (
            <div className="flex min-w-0 items-center gap-1.5">
              <p className="truncate text-[13px] font-medium text-ink">{name}</p>
              {current ? <CurrentTag status={status} /> : null}
              {onRename ? (
                <button
                  type="button"
                  onClick={() => setEditing(true)}
                  aria-label={`Rename ${name}`}
                  title="Rename"
                  className="grid size-5 shrink-0 place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                >
                  <Pencil size={11} />
                </button>
              ) : null}
            </div>
          )}
          {url ? <p className="mt-0.5 truncate font-mono text-[11px] text-ink-3">{url}</p> : null}
          <p className="mt-0.5 truncate text-[12px] text-ink-3">{detail}</p>
        </div>
      </div>
      <div className="flex items-center gap-1.5 md:justify-self-end">
        {onSwitch ? (
          <Button size="sm" variant="secondary" disabled={disabled} onClick={onSwitch}>
            {busy ? <LoaderCircle size={13} className="animate-spin" /> : null}
            Switch
          </Button>
        ) : null}
        {onForget ? (
          <Button size="sm" variant="danger" disabled={disabled} onClick={onForget}>
            <Trash2 size={13} />
            Forget
          </Button>
        ) : null}
      </div>
    </div>
  )
}

function CurrentTag({ status }: { status: ConnectionStatus }) {
  const { dot } = connectionStatusDisplay(status)
  return (
    <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-primary-soft px-1.5 py-0.5 text-[10px] font-medium text-primary-strong">
      <span className={`size-1 rounded-full ${dot}`} />
      {status === 'reconnecting' ? 'Reconnecting' : 'Connected'}
    </span>
  )
}
