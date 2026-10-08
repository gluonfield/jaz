import { useEffect, useState } from 'react'
import { Cookie, KeyRound, LoaderCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { Switch } from '@/components/ui/Switch'
import type { BrowserImportResult, BrowserProfile, BrowserProfileAPI } from '@shared/browserProfile'

export function BrowserProfileImportDialog({ api, onClose }: {
  api: BrowserProfileAPI
  onClose: () => void
}) {
  const [profiles, setProfiles] = useState<BrowserProfile[]>([])
  const [profileId, setProfileId] = useState('')
  const [cookies, setCookies] = useState(true)
  const [passwords, setPasswords] = useState(true)
  const [loading, setLoading] = useState(true)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<BrowserImportResult>()
  useEffect(() => {
    let cancelled = false
    void api.list().then((profiles) => {
      if (!cancelled) {
        setProfiles(profiles)
        setProfileId(profiles[0]?.id ?? '')
        setLoading(false)
      }
    }).catch((error: Error) => {
      if (!cancelled) {
        setError(error.message)
        setLoading(false)
      }
    })
    return () => {
      cancelled = true
    }
  }, [api])
  const profile = profiles.find((entry) => entry.id === profileId)
  const selection = { cookies: cookies && Boolean(profile?.cookies), passwords: passwords && Boolean(profile?.passwords) }
  const complete = result && Object.values(result).every((count) => !count.failed && !count.error)
  const startImport = async () => {
    setImporting(true)
    setError('')
    setResult(undefined)
    try {
      setResult(await api.import(profileId, selection))
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Import failed. Try again.')
    } finally {
      setImporting(false)
    }
  }
  const close = () => {
    if (!importing) {
      onClose()
    }
  }
  return <Modal open onClose={close} title="Import from your browser" chromeless className="rounded-[22px]!">
    <section className="bg-surface p-6">
      <h2 className="pr-7 text-[22px] font-semibold tracking-tight text-ink">Import from your browser</h2>
      <p className="mt-2 text-[13px] text-ink-2">Choose data to bring over to the built-in browser.</p>
      <label className="mt-6 flex items-center gap-4 text-[13px] text-ink-2">
        From
        <select aria-label="Browser profile" value={profileId} disabled={loading || importing || profiles.length === 0}
          onChange={(event) => {
            setProfileId(event.target.value)
            setError('')
            setResult(undefined)
          }}
          className="h-10 min-w-0 flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[14px] text-ink outline-none focus:border-primary disabled:opacity-50">
          {!profiles.length ? <option value="">{loading ? 'Reading browser profiles…' : 'No supported profiles found'}</option> : null}
          {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.browser} · {profile.name}</option>)}
        </select>
      </label>
      {profile ? <p className="mt-3 text-[13px] text-ink-2">Close {profile.browser} completely before importing.</p> : null}
      <div className="mt-5 divide-y divide-border rounded-2xl border border-border bg-bg/40 px-4">
        <div className="flex min-h-14 items-center gap-3">
          <KeyRound size={20} className="shrink-0 text-ink-2" />
          <span className="flex-1 text-[14px] text-ink">Saved passwords</span>
          <Switch className="before:absolute before:-inset-2.5" aria-label="Import saved passwords" checked={selection.passwords} disabled={importing || !profile?.passwords || Boolean(complete)} onChange={setPasswords} />
        </div>
        <div className="flex min-h-14 items-center gap-3">
          <Cookie size={20} className="shrink-0 text-ink-2" />
          <span className="flex-1 text-[14px] text-ink">Cookies</span>
          <Switch className="before:absolute before:-inset-2.5" aria-label="Import cookies" checked={selection.cookies} disabled={importing || !profile?.cookies || Boolean(complete)} onChange={setCookies} />
        </div>
      </div>
      <p className="mt-3 text-[12px] leading-relaxed text-ink-2">
        {profile?.passwords ? 'Imports HTTPS logins. Existing Jaz passwords are kept.' : 'Password import is available from Chrome and Edge on macOS.'}
      </p>
      {!loading && !profiles.length && !error ? <p className="mt-3 text-[13px] text-ink-2">Use Chrome or Edge on macOS, or Firefox for cookies.</p> : null}
      {importing ? <p role="status" className="mt-3 text-[12px] text-ink-2">Your computer may ask for access to the selected browser’s saved data.</p> : null}
      {result ? <div role="status" className="mt-4 space-y-1 text-[13px] text-ink">
        {(['passwords', 'cookies'] as const).map((kind) => {
          const count = result[kind]
          if (!count) {
            return null
          }
          return <p key={kind}>
            {count.error || `Imported ${count.imported} ${count.imported === 1 ? kind.slice(0, -1) : kind}.`}
            {count.skipped ? ` Kept ${count.skipped} existing or duplicate logins.` : ''}
            {count.failed ? ` ${count.failed} could not be imported.` : ''}
          </p>
        })}
      </div> : null}
      {error ? <p role="alert" className="mt-3 text-[13px] text-danger">{error}</p> : null}
      <div className="mt-6 flex justify-end gap-3">
        <Button className="min-h-10 rounded-xl! bg-surface-2 px-4" disabled={importing} onClick={onClose}>{result ? 'Done' : 'Cancel'}</Button>
        {!complete ? <Button variant="primary" className="min-h-10 rounded-xl! bg-ink! px-4 text-bg! hover:bg-ink/90!" disabled={importing || loading || (!selection.cookies && !selection.passwords)} onClick={() => void startImport()}>
          {importing ? <LoaderCircle size={14} className="animate-spin" /> : null}
          {importing ? 'Importing…' : 'Import'}
        </Button> : null}
      </div>
    </section>
  </Modal>
}
