import { Cookie, Download, KeyRound, MoreVertical, Trash2 } from 'lucide-react'
import { useCallback, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { BrowserDownloads } from '@/components/browser/BrowserDownloads'
import { useBrowserPasswords, type BrowserPasswordsController } from '@/components/browser/useBrowserPasswords'
import type { BrowserPasswordAction } from '@shared/browserPasswords'

export function BrowserMenu({
  webContentsId,
  visible,
  passwords: externalPasswords,
  onOpenImport,
}: {
  webContentsId: number | null
  visible: boolean
  passwords?: BrowserPasswordsController
  onOpenImport: () => void
}) {
  const internalPasswords = useBrowserPasswords(externalPasswords ? null : webContentsId)
  const passwords = externalPasswords ?? internalPasswords
  const { api, state, busy, error } = passwords
  const [view, setView] = useState<'menu' | 'passwords' | 'downloads' | null>(null)
  const close = useCallback(() => setView(null), [])

  if (!api) {
    return null
  }

  const act = async (action: BrowserPasswordAction) => {
    const next = await passwords.act(action)
    if (next && !next.error && (action.kind === 'save' || action.kind === 'dismiss' || action.kind === 'fill')) {
      close()
    }
  }

  const pending = state.pending
  const entries = state.entries?.length
    ? state.entries
    : state.usernames.map((username) => ({ origin: state.origin, username }))

  return (
    <Popover
      open={view !== null && visible}
      onClose={close}
      placement="below"
      align="end"
      trigger={<IconButton
        aria-label="Browser menu"
        title="Browser menu"
        aria-haspopup="menu"
        aria-expanded={view !== null && visible}
        onClick={() => setView((current) => current ? null : 'menu')}
        size="sm"
        className={`pointer-coarse:size-10 ${pending ? 'text-primary' : ''}`}
      ><MoreVertical size={15} /></IconButton>}
    >
      {view === 'menu' ? <>
        <MenuRow disabled={!window.jaz?.browserDownloads} onClick={() => setView('downloads')}>
          <span className="flex items-center gap-2"><Download size={13} />Downloads</span>
        </MenuRow>
        <MenuRow disabled={webContentsId === null} onClick={() => setView('passwords')}>
          <span className="flex items-center gap-2"><KeyRound size={13} />Passwords</span>
        </MenuRow>
        <MenuRow disabled={!window.jaz?.browserProfiles} onClick={() => {
          close()
          onOpenImport()
        }}>
          <span className="flex items-center gap-2"><Cookie size={13} />Import cookies and passwords</span>
        </MenuRow>
      </> : view === 'downloads' && window.jaz?.browserDownloads ? <BrowserDownloads api={window.jaz.browserDownloads} onBack={() => setView('menu')} /> : (
      <section aria-label="Browser passwords menu" className="w-72 max-w-[calc(100vw-32px)] p-2.5">
        <h2 className="text-[14px] font-medium text-ink">{pending ? pending.update ? 'Update password?' : 'Save password?' : 'Saved passwords'}</h2>
        {pending?.origin || state.origin ? <p className="mt-1 break-all text-[12px] text-ink-2">{pending?.origin || state.origin}</p> : null}
        {error || state.error ? <p role="alert" className="mt-2 text-[12px] text-danger">{error || state.error}</p> : null}
        {pending ? <>
          <p className="mt-3 break-all text-[13px] text-ink">{pending.username || 'No username'}</p>
          <p className="mt-1 text-[12px] text-ink-2">Save encrypts this login on your computer.</p>
          <div className="mt-4 flex justify-end gap-2">
            <Button size="sm" variant="ghost" disabled={busy} className="min-h-10" onClick={() => void act({ kind: 'dismiss', id: pending.id })}>Not now</Button>
            <Button size="sm" variant="primary" disabled={busy} className="min-h-10" onClick={() => void act({ kind: 'save', id: pending.id })}>{pending.update ? 'Update' : 'Save'}</Button>
          </div>
        </> : <>
          {state.locked ? <>
            <p className="mt-3 text-[13px] text-ink-2">Unlock to view and migrate previously saved logins for this site.</p>
            <Button size="sm" variant="primary" className="mt-3 min-h-10 w-full" disabled={busy} onClick={() => void act({ kind: 'unlock' })}>Unlock saved passwords</Button>
          </> : entries.length ? <div className="mt-3 max-h-60 overflow-y-auto">
            {entries.map((entry) => <div key={`${entry.origin}\0${entry.username}`} className="flex items-center gap-1">
              <button type="button" disabled={busy} className="min-h-10 min-w-0 flex-1 rounded-lg px-2 py-2 text-left text-[13px] text-ink hover:bg-surface-2" onClick={() => void act({ kind: 'fill', origin: entry.origin, username: entry.username })} title="Fill this login">
                <span className="block truncate">{entry.username || 'No username'}</span>
                <span className="block truncate text-[11px] text-ink-3">{entry.origin !== state.origin ? entry.origin : 'Fill login'}</span>
              </button>
              <IconButton aria-label={`Delete saved password for ${entry.username || 'this account'}`} disabled={busy} className="size-10" onClick={() => void act({ kind: 'remove', origin: entry.origin, username: entry.username })}><Trash2 size={14} /></IconButton>
            </div>)}
          </div> : <p className="mt-3 text-[13px] text-ink-2">No saved logins for this site.</p>}
          {state.origin ? <Button size="sm" variant="ghost" className="mt-3 min-h-10 w-full" disabled={busy} onClick={() => void act({ kind: 'capture' })}>Save login on this page</Button> : null}
        </>}
      </section>
      )}
    </Popover>
  )
}
