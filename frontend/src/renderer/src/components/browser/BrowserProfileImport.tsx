import { useEffect, useState } from 'react'
import { Cookie, X } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { BrowserProfileImportDialog } from '@/components/browser/BrowserProfileImportDialog'

export function BrowserProfileImport({ offer = false }: { offer?: boolean }) {
  const api = window.jaz?.browserProfiles
  const [visible, setVisible] = useState(!offer)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!api || !offer) {
      return
    }
    let cancelled = false
    void api.dismissed().then((dismissed) => {
      if (!cancelled) {
        setVisible(!dismissed)
      }
    }).catch(() => {})
    return () => {
      cancelled = true
    }
  }, [api, offer])
  if (!api) {
    return null
  }
  const dismiss = async () => {
    try {
      await api.dismiss()
      setVisible(false)
    } catch {
      setError('Could not save this choice. Try again.')
    }
  }
  return <>
    {visible && offer ? (
      <div className="flex shrink-0 items-center gap-3 border-b border-border bg-surface px-3 py-2" data-browser-profile-offer>
        <Cookie size={18} className="shrink-0 text-ink-2" />
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium text-ink">Stay signed in to your sites</p>
          <p className="text-[12px] text-ink-2">Import sign-ins from your browser.</p>
          {error ? <p role="alert" className="text-[12px] text-danger">{error}</p> : null}
        </div>
        <Button size="sm" className="min-h-10" onClick={() => setOpen(true)}>Import sign-ins</Button>
        <button type="button" className="grid size-10 shrink-0 place-items-center rounded-full text-ink-3 hover:bg-surface-2" aria-label="Dismiss import suggestion" onClick={() => void dismiss()}>
          <X size={15} />
        </button>
      </div>
    ) : !offer ? (
      <Button className="min-h-10" onClick={() => setOpen(true)}><Cookie size={15} />Import sign-ins</Button>
    ) : null}
    {open ? <BrowserProfileImportDialog api={api} onClose={() => setOpen(false)} onImported={() => setVisible(false)} /> : null}
  </>
}
