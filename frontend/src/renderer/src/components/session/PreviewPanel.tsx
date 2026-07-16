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
import { previewDisplayUrl, resolvePreviewSource, shouldProxyPreview } from '@/lib/api/preview'
import type { Attachment } from '@/lib/api/types'
import { clientRuntime } from '@/lib/clientRuntime'
import type { BrowserAnnotation } from '@/lib/messageContext'
import { normalizePreviewURL } from '../../../../shared/preview'
import {
  captureBrowserAnnotation,
  clearBrowserAnnotationCapture,
  isBrowserAnnotationCancelled,
} from './browserAnnotationCapture'
import { PreviewFindBar } from './PreviewFindBar'
import { SidePanelShell } from './SidePanelShell'
import {
  isPreviewWebviewPending,
  previewWebviewErrorMessage,
  type PreviewNavigationEvent,
  type PreviewWebviewElement,
} from './previewWebview'
import type { PreviewTarget } from './previewTarget'
import { usePreviewFindControls } from './usePreviewFindControls'

export const PREVIEW_PANEL_WIDTH = 640

export function PreviewPanel({
  target,
  onTargetChange,
  onAddBrowserAnnotation,
  onUploadAttachment,
  onClose,
}: {
  target: PreviewTarget
  onTargetChange: (target: PreviewTarget) => void
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onClose: () => void
}) {
  const webviewRef = useRef<PreviewWebviewElement | null>(null)
  const readyRef = useRef(false)
  const targetRef = useRef(target)
  const canUseWebview = clientRuntime.capabilities.previewWebview
  const [webview, setWebview] = useState<PreviewWebviewElement | null>(null)
  const [draft, setDraft] = useState(target.displayUrl)
  const [resolvedSourceUrl, setResolvedSourceUrl] = useState(target.sourceUrl)
  const [webviewReady, setWebviewReady] = useState(false)
  const [iframeKey, setIframeKey] = useState(0)
  const [loading, setLoading] = useState(false)
  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)
  const [annotating, setAnnotating] = useState(false)
  const [error, setError] = useState('')
  const find = usePreviewFindControls({
    webview,
    webviewReady,
    canUseWebview,
    onError: setError,
  })

  useEffect(() => {
    targetRef.current = target
    setDraft(target.displayUrl)
    setError('')
    if (!canUseWebview) {
      setLoading(Boolean(target.sourceUrl))
      setWebviewReady(Boolean(target.sourceUrl))
      setCanGoBack(false)
      setCanGoForward(false)
    }
  }, [canUseWebview, target])

  useEffect(() => {
    let cancelled = false
    setResolvedSourceUrl(shouldProxyPreview(target.sourceUrl) ? '' : target.sourceUrl)
    if (!target.sourceUrl) return
    void resolvePreviewSource(target.sourceUrl)
      .then((source) => {
        if (!cancelled) setResolvedSourceUrl(source)
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setLoading(false)
          setWebviewReady(false)
          setError(err.message || 'Preview failed to load.')
        }
      })
    return () => {
      cancelled = true
    }
  }, [target.sourceUrl])

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
          if (!isPreviewWebviewPending(err)) setError(previewWebviewErrorMessage(err))
        }
      }
      if (next) {
        const display = previewDisplayUrl(next) ?? next
        setDraft(display)
        if (display !== targetRef.current.displayUrl || next !== targetRef.current.sourceUrl) {
          targetRef.current = { displayUrl: display, sourceUrl: next }
          onTargetChange(targetRef.current)
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
  }, [onTargetChange, resolvedSourceUrl, webview])

  const openDraft = () => {
    const next = normalizePreviewURL(draft)
    if (!next) {
      setError('Enter an http or https URL.')
      return
    }
    setError('')
    targetRef.current = { displayUrl: next, sourceUrl: next }
    onTargetChange(targetRef.current)
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
      if (!isPreviewWebviewPending(err)) setError(previewWebviewErrorMessage(err))
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
      if (!isBrowserAnnotationCancelled(err)) setError(previewWebviewErrorMessage(err))
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
    setLoading(Boolean(resolvedSourceUrl))
    setIframeKey((key) => key + 1)
  }

  const canAnnotate = canUseWebview && !!onAddBrowserAnnotation

  return (
    <SidePanelShell width={PREVIEW_PANEL_WIDTH} onKeyDownCapture={find.handleKeyDownCapture}>
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
          disabled={!resolvedSourceUrl || (canUseWebview && !webviewReady)}
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
          disabled={!resolvedSourceUrl}
          onClick={() => window.open(resolvedSourceUrl, '_blank', 'noopener')}
        >
          <ExternalLink size={14} />
        </IconButton>
        <IconButton
          size="sm"
          aria-label={annotating ? 'Stop annotation' : 'Annotate preview'}
          title={annotating ? 'Stop annotation' : 'Annotate'}
          disabled={!resolvedSourceUrl || !webviewReady || !canAnnotate}
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
      <div className="relative min-h-0 flex-1 bg-bg">
        <PreviewFindBar find={find} />
        {resolvedSourceUrl && canUseWebview ? (
          <webview
            ref={bindWebview}
            src={resolvedSourceUrl}
            partition="persist:jaz-preview"
            allowpopups
            className="h-full w-full bg-bg"
          />
        ) : resolvedSourceUrl ? (
          <iframe
            key={iframeKey}
            src={resolvedSourceUrl}
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
