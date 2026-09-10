import { useEffect, useState } from 'react'
import { Check, CircleAlert, Cookie, LoaderCircle, Search } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { BrowserCookieSite, BrowserImportResult, BrowserProfile, BrowserProfileAPI } from '@shared/browserProfile'

export function BrowserProfileImportDialog({ api, onClose, onImported }: {
  api: BrowserProfileAPI
  onClose: () => void
  onImported: () => void
}) {
  const [profiles, setProfiles] = useState<BrowserProfile[]>([])
  const [profileId, setProfileId] = useState('')
  const [sites, setSites] = useState<BrowserCookieSite[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<BrowserImportResult>()
  useEffect(() => {
    let cancelled = false
    void api.list().then((profiles) => {
      if (!cancelled) {
        setProfiles(profiles)
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
  useEffect(() => {
    if (!profileId) {
      return
    }
    let cancelled = false
    void api.sites(profileId).then((sites) => {
      if (!cancelled) {
        setSites(sites)
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
  }, [api, profileId])
  const chooseProfile = (id: string) => {
    setProfileId(id)
    setSites([])
    setSelected(new Set())
    setQuery('')
    setError('')
    setResult(undefined)
    setLoading(Boolean(id))
  }
  const startImport = async () => {
    setImporting(true)
    setError('')
    try {
      const result = await api.import(profileId, [...selected])
      setResult(result)
      if (result.imported > 0) {
        onImported()
      }
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Import failed. Try again.')
    } finally {
      setImporting(false)
    }
  }
  const visibleSites = sites.filter((site) => site.domain.toLowerCase().includes(query.toLowerCase()))
  return (
    <Modal open onClose={() => {
      if (!importing) {
        onClose()
      }
    }} title="Import browser sign-ins" icon={<Cookie size={18} />}
      description="Choose a browser profile and the sites you want to use in Jaz."
      footer={<>
        <Button className="min-h-10" disabled={importing} onClick={onClose}>{result ? 'Done' : 'Cancel'}</Button>
        <Button variant="primary" className="min-h-10" disabled={importing || loading || selected.size === 0 || Boolean(result && !result.failed)} onClick={() => void startImport()}>
          {importing ? <LoaderCircle size={14} className="animate-spin" /> : null}
          {importing ? 'Importing…' : selected.size ? `Import ${selected.size} ${selected.size === 1 ? 'site' : 'sites'}` : 'Import sign-ins'}
        </Button>
      </>}
    >
      <div className="space-y-4">
        <p className="text-[13px] leading-relaxed text-ink-2">
          Selected sign-ins stay on this computer and are available to agents using its side browser. Your source browser stays unchanged.
        </p>
        <label className="block text-[12px] font-medium text-ink">
          Browser profile
          <select aria-label="Browser profile" value={profileId} disabled={importing || profiles.length === 0}
            onChange={(event) => chooseProfile(event.target.value)}
            className="mt-1.5 h-10 w-full rounded-control border border-border bg-surface px-3 text-[13px] text-ink outline-none focus:border-primary disabled:opacity-50">
            <option value="">Choose a profile</option>
            {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.browser} · {profile.name}</option>)}
          </select>
        </label>
        {!loading && profiles.length === 0 && !error ? <p className="text-[13px] text-ink-2">No supported profiles found. Use Chrome or Edge on macOS, or Firefox on macOS, Windows, or Linux. You can also sign in directly in Jaz.</p> : null}
        {loading ? <p role="status" className="flex items-center gap-2 text-[13px] text-ink-2"><LoaderCircle size={14} className="animate-spin" />{profileId ? 'Reading sites…' : 'Reading browser profiles…'}</p> : null}
        {sites.length > 0 ? <>
          <label className="flex h-10 items-center gap-2 rounded-control bg-surface px-3 ring-1 ring-border focus-within:ring-primary">
            <Search size={15} className="text-ink-3" />
            <input aria-label="Search sites" placeholder="Search sites" value={query} disabled={importing} onChange={(event) => setQuery(event.target.value)} className="min-w-0 flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-ink-3" />
          </label>
          <div className="max-h-60 overflow-y-auto rounded-control ring-1 ring-border">
            {visibleSites.map((site) => <label key={site.domain} className="flex min-h-11 cursor-pointer items-center gap-3 px-3 py-2 hover:bg-surface-2">
              <input type="checkbox" checked={selected.has(site.domain)} disabled={importing || Boolean(result && !result.failed)} onChange={(event) => setSelected((current) => {
                const next = new Set(current)
                if (event.target.checked) {
                  next.add(site.domain)
                } else {
                  next.delete(site.domain)
                }
                return next
              })} className="size-4 shrink-0 accent-primary" />
              <span className="min-w-0 flex-1 break-all text-[13px] text-ink">{site.domain}</span>
              <span className="shrink-0 text-[11px] tabular-nums text-ink-3">{site.cookies} {site.cookies === 1 ? 'cookie' : 'cookies'}</span>
            </label>)}
            {visibleSites.length === 0 ? <p className="px-3 py-4 text-[13px] text-ink-2">No matching sites.</p> : null}
          </div>
        </> : profileId && !loading && !error ? <p className="text-[13px] text-ink-2">No importable cookies found in this profile.</p> : null}
        {importing ? <p role="status" className="text-[12px] text-ink-2">Your computer may ask you to allow access to the selected browser’s sign-ins.</p> : null}
        {result ? <p role="status" className="flex items-start gap-2 text-[13px] text-ink">
          {result.failed > 0 ? <CircleAlert size={16} className="mt-0.5 shrink-0" /> : <Check size={16} className="mt-0.5 shrink-0" />}
          <span>Imported {result.imported} {result.imported === 1 ? 'cookie' : 'cookies'}.{result.failed > 0 ? ` ${result.failed} could not be imported.` : ''} Some sites may still ask you to sign in.</span>
        </p> : null}
        {error ? <p role="alert" className="text-[13px] text-danger">{error}</p> : null}
      </div>
    </Modal>
  )
}
