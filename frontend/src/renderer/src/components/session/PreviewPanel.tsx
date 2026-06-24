import {
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Globe,
  LoaderCircle,
  MessageSquare,
  RotateCw,
  SquareStop,
  X,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import type { Attachment } from '@/lib/api/types'
import { clientRuntime } from '@/lib/clientRuntime'
import type { BrowserAnnotation } from '@/lib/messageContext'
import { normalizePreviewURL } from '../../../../shared/preview'
import {
  captureBrowserAnnotation,
  clearBrowserAnnotationCapture,
  isBrowserAnnotationCancelled,
} from './browserAnnotationCapture'
import { SidePanelShell } from './SidePanelShell'
import type { PreviewNavigationEvent, PreviewWebviewElement } from './previewWebview'

export const PREVIEW_PANEL_WIDTH = 640

export function PreviewPanel({
  url,
  onUrlChange,
  onAddBrowserAnnotation,
  onUploadAttachment,
  onClose,
}: {
  url: string
  onUrlChange: (url: string) => void
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onClose: () => void
}) {
  const webviewRef = useRef<PreviewWebviewElement | null>(null)
  const readyRef = useRef(false)
  const urlRef = useRef(url)
  const canUseWebview = clientRuntime.capabilities.previewWebview
  const [webview, setWebview] = useState<PreviewWebviewElement | null>(null)
  const [draft, setDraft] = useState(url)
  const [webviewReady, setWebviewReady] = useState(false)
  const [iframeKey, setIframeKey] = useState(0)
  const [loading, setLoading] = useState(false)
  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)
  const [annotating, setAnnotating] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    urlRef.current = url
    setDraft(url)
    setError('')
    if (!canUseWebview) {
      setLoading(Boolean(url))
      setWebviewReady(Boolean(url))
      setCanGoBack(false)
      setCanGoForward(false)
    }
  }, [canUseWebview, url])

  const bindWebview = useCallback((element: Element | null) => {
    const next = element as PreviewWebviewElement | null
    webviewRef.current = next
    setWebview(next)
  }, [])

  useEffect(() => {
    if (!webview) return
    readyRef.current = false
    setWebviewReady(false)
    setCanGoBack(false)
    setCanGoForward(false)
    const sync = (event?: PreviewNavigationEvent) => {
      let next = event?.url || event?.validatedURL || webview.src
      if (readyRef.current) {
        try {
          next = event?.url || event?.validatedURL || webview.getURL() || webview.src
          setCanGoBack(webview.canGoBack())
          setCanGoForward(webview.canGoForward())
        } catch (err) {
          readyRef.current = false
          setWebviewReady(false)
          setCanGoBack(false)
          setCanGoForward(false)
          if (!isWebviewPending(err)) setError(webviewErrorMessage(err))
        }
      }
      if (next) {
        setDraft(next)
        if (next !== urlRef.current) {
          urlRef.current = next
          onUrlChange(next)
        }
      }
    }
    const ready = () => {
      readyRef.current = true
      setWebviewReady(true)
      sync()
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
    webview.addEventListener('dom-ready', ready)
    return () => {
      webview.removeEventListener('did-start-loading', start)
      webview.removeEventListener('did-stop-loading', stop)
      webview.removeEventListener('did-navigate', sync as EventListener)
      webview.removeEventListener('did-navigate-in-page', sync as EventListener)
      webview.removeEventListener('did-fail-load', fail as EventListener)
      webview.removeEventListener('dom-ready', ready)
    }
  }, [onUrlChange, webview])

  const openDraft = () => {
    const next = normalizePreviewURL(draft)
    if (!next) {
      setError('Enter an http or https URL.')
      return
    }
    setError('')
    urlRef.current = next
    onUrlChange(next)
  }

  const runWhenReady = (action: (webview: PreviewWebviewElement) => void) => {
    const webview = webviewRef.current
    if (!webview || !webviewReady) return
    try {
      action(webview)
    } catch (err) {
      readyRef.current = false
      setWebviewReady(false)
      setCanGoBack(false)
      setCanGoForward(false)
      if (!isWebviewPending(err)) setError(webviewErrorMessage(err))
    }
  }

  const annotate = async () => {
    const webview = webviewRef.current
    if (!webview || !webviewReady || annotating || !onAddBrowserAnnotation) return
    setAnnotating(true)
    setError('')
    try {
      const capture = await captureBrowserAnnotation(webview, onUploadAttachment)
      if (capture) onAddBrowserAnnotation(capture.annotation, capture.screenshot)
    } catch (err) {
      if (!isBrowserAnnotationCancelled(err)) setError(webviewErrorMessage(err))
    } finally {
      setAnnotating(false)
      await clearBrowserAnnotationCapture(webview)
    }
  }

  const stopAnnotation = async () => {
    const webview = webviewRef.current
    if (!webview || !annotating) return
    await clearBrowserAnnotationCapture(webview)
  }

  const reload = () => {
    if (canUseWebview) {
      runWhenReady((view) => view.reload())
      return
    }
    setLoading(Boolean(url))
    setIframeKey((key) => key + 1)
  }

  const canAnnotate = canUseWebview && !!onAddBrowserAnnotation

  return (
    <SidePanelShell width={PREVIEW_PANEL_WIDTH}>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          openDraft()
        }}
        className="flex h-11 shrink-0 items-center gap-1.5 border-b border-border px-2.5"
      >
        <IconButton
          size="sm"
          aria-label="Back"
          title="Back"
          disabled={!webviewReady || !canGoBack}
          onClick={() => runWhenReady((view) => view.goBack())}
        >
          <ArrowLeft size={14} />
        </IconButton>
        <IconButton
          size="sm"
          aria-label="Forward"
          title="Forward"
          disabled={!webviewReady || !canGoForward}
          onClick={() => runWhenReady((view) => view.goForward())}
        >
          <ArrowRight size={14} />
        </IconButton>
        <IconButton
          size="sm"
          aria-label="Reload preview"
          title="Reload"
          disabled={!url || (canUseWebview && !webviewReady)}
          onClick={reload}
        >
          {loading ? <LoaderCircle size={14} className="animate-spin" /> : <RotateCw size={14} />}
        </IconButton>
        <div className="flex min-w-0 flex-1 items-center gap-1.5 rounded-[8px] bg-bg/60 px-2.5 py-1.5 ring-1 ring-border/70">
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
          aria-label="Open in Browser"
          title="Open in Browser"
          disabled={!url}
          onClick={() => window.open(url, '_blank', 'noopener')}
        >
          <ExternalLink size={14} />
        </IconButton>
        <IconButton
          size="sm"
          aria-label={annotating ? 'Stop annotation' : 'Annotate preview'}
          title={annotating ? 'Stop annotation' : 'Annotate'}
          disabled={!url || !webviewReady || !canAnnotate}
          onClick={() => (annotating ? void stopAnnotation() : void annotate())}
          className={annotating ? 'text-danger hover:bg-danger-soft hover:text-danger' : ''}
        >
          {annotating ? <SquareStop size={14} /> : <MessageSquare size={14} />}
        </IconButton>
        <button
          type="button"
          aria-label="Hide side panel"
          onClick={onClose}
          className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-[8px] text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
        >
          <X size={15} />
        </button>
      </form>
      {error ? (
        <p className="shrink-0 border-b border-border px-3 py-2 text-[12px] text-danger">{error}</p>
      ) : null}
      <div className="min-h-0 flex-1 bg-bg">
        {url && canUseWebview ? (
          <webview
            ref={bindWebview}
            src={url}
            partition="persist:jaz-preview"
            allowpopups
            className="h-full w-full bg-bg"
          />
        ) : url ? (
          <iframe
            key={iframeKey}
            src={url}
            title="Preview"
            sandbox="allow-downloads allow-forms allow-modals allow-popups allow-popups-to-escape-sandbox allow-same-origin allow-scripts"
            referrerPolicy="no-referrer"
            onLoad={() => {
              setLoading(false)
              setWebviewReady(true)
            }}
            className="h-full w-full border-0 bg-bg"
          />
        ) : (
          <div className="flex h-full items-center justify-center px-8 text-center text-[13px] text-ink-3">
            No preview selected.
          </div>
        )}
      </div>
    </SidePanelShell>
  )
}

function isWebviewPending(error: unknown): boolean {
  const message = webviewErrorMessage(error)
  return message.includes('WebView must be attached') || message.includes('dom-ready')
}

function webviewErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
