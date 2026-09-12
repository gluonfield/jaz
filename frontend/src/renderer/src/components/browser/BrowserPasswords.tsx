import { KeyRound, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Popover } from '@/components/ui/Popover'
import type { BrowserPasswordAction, BrowserPasswordState } from '@shared/browserPasswords'

export function BrowserPasswords({ webContentsId, visible }: { webContentsId: number | null; visible: boolean }) {
  const api = window.jaz?.browserPasswords
  const [state, setState] = useState<BrowserPasswordState>({ origin: '', usernames: [] })
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const revision = useRef(0)
  const close = useCallback(() => setOpen(false), [])

  useEffect(() => {
    if (!api || webContentsId === null) {
      return
    }
    let active = true
    const refresh = async () => {
      const request = ++revision.current
      try {
        const next = await api.state(webContentsId)
        if (active && request === revision.current) {
          setState(next)
        }
      } catch {
        if (active) {
          setError('Passwords could not be loaded.')
        }
      }
    }
    void refresh()
    const unsubscribe = api.subscribe(webContentsId, () => void refresh())
    return () => {
      active = false
      unsubscribe()
    }
  }, [api, webContentsId])

  useEffect(() => {
    setOpen(Boolean(state.pending?.id || (state.origin && state.error)))
  }, [state.pending?.id, state.origin, state.error])

  if (!api) {
    return null
  }

  const act = async (action: BrowserPasswordAction) => {
    if (webContentsId === null) {
      return
    }
    const request = ++revision.current
    setBusy(true)
    setError('')
    try {
      const next = await api.act(webContentsId, action)
      if (request !== revision.current) {
        return
      }
      setState(next)
      if (!next.error && action.kind !== 'remove' && action.kind !== 'capture') {
        close()
      }
    } catch {
      setError('The password action failed. Try again.')
    } finally {
      setBusy(false)
    }
  }

  const pending = state.pending
  return <Popover
    open={open && visible && webContentsId !== null}
    onClose={close}
    placement="below"
    align="end"
    trigger={<IconButton
      size="sm"
      aria-label="Passwords"
      title="Saved passwords"
      aria-expanded={open && visible}
      disabled={webContentsId === null}
      onClick={() => setOpen((value) => !value)}
      className={pending ? 'text-primary' : ''}
    ><KeyRound size={15} /></IconButton>}
  >
    <section aria-label="Browser passwords" className="w-72 max-w-[calc(100vw-32px)] p-2.5">
      <h2 className="text-[14px] font-medium text-ink">{pending ? pending.update ? 'Update password?' : 'Save password?' : 'Saved passwords'}</h2>
      {state.origin ? <p className="mt-1 break-all text-[12px] text-ink-2">{state.origin}</p> : null}
      {error || state.error ? <p role="alert" className="mt-2 text-[12px] text-danger">{error || state.error}</p> : null}
      {pending ? <>
        <p className="mt-3 break-all text-[13px] text-ink">{pending.username || 'No username'}</p>
        <p className="mt-1 text-[12px] text-ink-2">Encrypted on this computer.</p>
        <div className="mt-4 flex justify-end gap-2">
          <Button size="sm" variant="ghost" disabled={busy} className="min-h-10" onClick={() => void act({ kind: 'dismiss', id: pending.id })}>Not now</Button>
          <Button size="sm" variant="primary" disabled={busy} className="min-h-10" onClick={() => void act({ kind: 'save', id: pending.id })}>{pending.update ? 'Update' : 'Save'}</Button>
        </div>
      </> : <>
        {state.usernames.length ? <div className="mt-3 max-h-60 overflow-y-auto">
          {state.usernames.map((username) => <div key={username} className="flex items-center gap-1">
            <button type="button" disabled={busy} className="min-h-10 min-w-0 flex-1 rounded-lg px-2 py-2 text-left text-[13px] text-ink hover:bg-surface-2" onClick={() => void act({ kind: 'fill', origin: state.origin, username })} title="Fill this login">
              <span className="block truncate">{username || 'No username'}</span>
              <span className="text-[11px] text-ink-3">Fill login</span>
            </button>
            <IconButton aria-label={`Delete saved password for ${username || 'this account'}`} disabled={busy} className="size-10" onClick={() => void act({ kind: 'remove', origin: state.origin, username })}><Trash2 size={14} /></IconButton>
          </div>)}
        </div> : <p className="mt-3 text-[13px] text-ink-2">No saved logins for this site.</p>}
        {state.origin ? <Button size="sm" variant="ghost" className="mt-3 min-h-10 w-full" disabled={busy} onClick={() => void act({ kind: 'capture' })}>Save login on this page</Button> : null}
      </>}
    </section>
  </Popover>
}
