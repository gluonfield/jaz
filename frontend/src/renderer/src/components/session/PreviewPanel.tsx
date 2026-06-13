import {
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Globe,
  LoaderCircle,
  RotateCw,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { normalizePreviewURL } from '../../../../shared/preview'
import type { PreviewNavigationEvent, PreviewWebviewElement } from './previewWebview'

export const PREVIEW_PANEL_WIDTH = 640

export function PreviewPanel({
  url,
  onUrlChange,
}: {
  url: string
  onUrlChange: (url: string) => void
}) {
  const webviewRef = useRef<PreviewWebviewElement | null>(null)
  const [draft, setDraft] = useState(url)
  const [loading, setLoading] = useState(false)
  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    setDraft(url)
    setError('')
  }, [url])

  useEffect(() => {
    const webview = webviewRef.current
    if (!webview) return
    const sync = (event?: PreviewNavigationEvent) => {
      const next = event?.url || event?.validatedURL || webview.getURL() || webview.src
      if (next) {
        setDraft(next)
        if (next !== url) onUrlChange(next)
      }
      setCanGoBack(webview.canGoBack())
      setCanGoForward(webview.canGoForward())
    }
    const start = () => {
      setLoading(true)
      setError('')
    }
    const stop = () => {
      setLoading(false)
      sync()
    }
    const fail = (event: PreviewNavigationEvent) => {
      setLoading(false)
      if (event.errorCode === -3 || event.isMainFrame === false) return
      setError(event.errorDescription || 'Preview failed to load.')
      sync(event)
    }
    webview.addEventListener('did-start-loading', start)
    webview.addEventListener('did-stop-loading', stop)
    webview.addEventListener('did-navigate', sync as EventListener)
    webview.addEventListener('did-navigate-in-page', sync as EventListener)
    webview.addEventListener('did-fail-load', fail as EventListener)
    sync()
    return () => {
      webview.removeEventListener('did-start-loading', start)
      webview.removeEventListener('did-stop-loading', stop)
      webview.removeEventListener('did-navigate', sync as EventListener)
      webview.removeEventListener('did-navigate-in-page', sync as EventListener)
      webview.removeEventListener('did-fail-load', fail as EventListener)
    }
  }, [onUrlChange, url])

  const openDraft = () => {
    const next = normalizePreviewURL(draft)
    if (!next) {
      setError('Enter an http or https URL.')
      return
    }
    setError('')
    onUrlChange(next)
  }

  return (
    <aside
      style={{ width: PREVIEW_PANEL_WIDTH }}
      className="flex h-full shrink-0 flex-col border-l border-border bg-bg"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          openDraft()
        }}
        className="flex h-12 shrink-0 items-center gap-1.5 border-b border-border px-3"
      >
        <IconButton
          size="sm"
          aria-label="Back"
          title="Back"
          disabled={!canGoBack}
          onClick={() => webviewRef.current?.goBack()}
        >
          <ArrowLeft size={14} />
        </IconButton>
        <IconButton
          size="sm"
          aria-label="Forward"
          title="Forward"
          disabled={!canGoForward}
          onClick={() => webviewRef.current?.goForward()}
        >
          <ArrowRight size={14} />
        </IconButton>
        <IconButton
          size="sm"
          aria-label="Reload preview"
          title="Reload"
          disabled={!url}
          onClick={() => webviewRef.current?.reload()}
        >
          {loading ? <LoaderCircle size={14} className="animate-spin" /> : <RotateCw size={14} />}
        </IconButton>
        <div className="flex min-w-0 flex-1 items-center gap-1.5 rounded-full bg-surface px-2.5 py-1.5">
          <Globe size={13} className="shrink-0 text-ink-3" aria-hidden />
          <input
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="https://localhost:3000"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent font-mono text-[12px] text-ink outline-none placeholder:text-ink-3"
          />
        </div>
        <IconButton
          size="sm"
          aria-label="Open preview externally"
          title="Open externally"
          disabled={!url}
          onClick={() => window.open(url, '_blank', 'noopener')}
        >
          <ExternalLink size={14} />
        </IconButton>
      </form>
      {error ? (
        <p className="shrink-0 border-b border-border px-4 py-2 text-[12px] text-danger">{error}</p>
      ) : null}
      <div className="min-h-0 flex-1 bg-surface">
        {url ? (
          <webview
            ref={(element) => {
              webviewRef.current = element as PreviewWebviewElement | null
            }}
            src={url}
            partition="persist:jaz-preview"
            allowpopups
            className="h-full w-full bg-bg"
          />
        ) : (
          <div className="flex h-full items-center justify-center px-8 text-center text-[13px] text-ink-3">
            Open an http or https link from chat, or enter a URL above.
          </div>
        )}
      </div>
    </aside>
  )
}
