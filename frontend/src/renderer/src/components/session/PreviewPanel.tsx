import {
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Globe,
  LoaderCircle,
  MessageCirclePlus,
  RotateCw,
  SquareStop,
  X,
} from 'lucide-react'
import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { previewDisplayUrl, resolvePreviewSource } from '@/lib/api/preview'
import type { Attachment } from '@/lib/api/types'
import { clientRuntime, openExternal } from '@/lib/clientRuntime'
import type { BrowserAnnotation } from '@/lib/messageContext'
import { isPreviewURL, normalizePreviewURL, shouldProxyPreview } from '@shared/preview'
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
import type { PreviewTarget } from '@/lib/browserSessions'
import type { SideBrowser } from '@/lib/sideBrowser'
import { BrowserMenu } from '@/components/browser/BrowserMenu'
import { PREVIEW_PARTITION } from '@shared/preview'
import { usePreviewFindControls } from './usePreviewFindControls'

export const PREVIEW_PANEL_WIDTH = 800

export function PreviewPanel({
  visible = true,
  browserControl,
  target,
  reloads,
  onTargetChange,
  onAddBrowserAnnotation,
  onUploadAttachment,
  onClose,
  embedded = false,
}: {
  visible?: boolean
  browserControl?: SideBrowser
  target: PreviewTarget
  reloads?: number
  onTargetChange: (target: PreviewTarget) => void
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onClose?: () => void
  embedded?: boolean
}) {
  const webviewRef = useRef<PreviewWebviewElement | null>(null)
  const cursorLayer = useRef<HTMLDivElement | null>(null)
  const readyRef = useRef<PreviewWebviewElement | null>(null)
  const targetRef = useRef(target)
  const canUseWebview = clientRuntime.capabilities.previewWebview
  const [webview, setWebview] = useState<PreviewWebviewElement | null>(null)
  const [draft, setDraft] = useState(target.displayUrl)
  const [resolvedSourceUrl, setResolvedSourceUrl] = useState(isPreviewURL(target.sourceUrl) ? target.sourceUrl : '')
  const [readyWebview, setReadyWebview] = useState<PreviewWebviewElement | null>(null)
  const webviewReady = webview !== null && readyWebview === webview
  const [iframeKey, setIframeKey] = useState(0)
  const [loading, setLoading] = useState(false)
  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)
  const [annotating, setAnnotating] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!visible || !webview || !annotating) return
    return () => {
      void clearBrowserAnnotationCapture(webview)
    }
  }, [visible, webview, annotating])
  useEffect(() => {
    if (!browserControl || !webview || !webviewReady || !cursorLayer.current) {
      return
    }
    return browserControl.attach(webview, cursorLayer.current)
  }, [browserControl, webview, webviewReady])
  const find = usePreviewFindControls({
    visible,
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
      setCanGoBack(false)
      setCanGoForward(false)
    }
  }, [canUseWebview, target])

  useEffect(() => {
    let cancelled = false
    if (readyRef.current === webviewRef.current && webviewRef.current?.getURL() === target.sourceUrl) {
      setResolvedSourceUrl(target.sourceUrl)
      return
    }
    setResolvedSourceUrl(isPreviewURL(target.sourceUrl) && !shouldProxyPreview(target.sourceUrl) ? target.sourceUrl : '')
    if (!target.sourceUrl) return
    void resolvePreviewSource(target.sourceUrl)
      .then((source) => {
        if (!cancelled) setResolvedSourceUrl(source)
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setLoading(false)
          setReadyWebview(null)
          setError(err.message || 'Preview failed to load.')
        }
      })
    return () => {
      cancelled = true
    }
  }, [target.sourceUrl])

  const bindWebview = useCallback((element: Element | null) => {
    // React omits this Electron boolean JSX attribute; set it before the guest loads.
    element?.setAttribute('allowpopups', '')
    const next = element as PreviewWebviewElement | null
    webviewRef.current = next
    setWebview(next)
  }, [])

  useEffect(() => {
    if (!webview) return
    const sync = (event?: PreviewNavigationEvent) => {
      if (event?.isMainFrame === false) {
        return
      }
      let next = event?.url || event?.validatedURL || webview.src
      if (readyRef.current === webview) {
        try {
          next = event?.url || event?.validatedURL || webview.getURL() || webview.src
          setCanGoBack(webview.canGoBack())
          setCanGoForward(webview.canGoForward())
        } catch (err) {
          readyRef.current = null
          setReadyWebview(null)
          setCanGoBack(false)
          setCanGoForward(false)
          if (!isPreviewWebviewPending(err)) setError(previewWebviewErrorMessage(err))
        }
      }
      if (next) {
        const display = previewDisplayUrl(next) ?? next
        setDraft(display)
        const title = readyRef.current === webview ? webview.getTitle() : undefined
        if (display !== targetRef.current.displayUrl || next !== targetRef.current.sourceUrl || title !== targetRef.current.title) {
          targetRef.current = { ...targetRef.current, displayUrl: display, sourceUrl: next, title }
          onTargetChange(targetRef.current)
        }
      }
    }
    const ready = () => {
      readyRef.current = webview
      setReadyWebview(webview)
      sync()
    }
    const start = () => {
      setLoading(true)
      setError('')
    }
    const favicon = (event: Event) => {
      const icons = (event as Event & { favicons: string[] }).favicons
      targetRef.current = { ...targetRef.current, favicon: icons.find((url) => /^(https?:\/\/|data:image\/)/i.test(url)) }
      onTargetChange(targetRef.current)
    }
    const navigate = (event: PreviewNavigationEvent) => {
      if (event.isMainFrame && !event.isInPlace && targetRef.current.favicon) {
        targetRef.current = { ...targetRef.current, favicon: undefined }
        onTargetChange(targetRef.current)
      }
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
    webview.addEventListener('did-start-navigation', navigate as EventListener)
    webview.addEventListener('page-favicon-updated', favicon)
    webview.addEventListener('did-stop-loading', stop)
    webview.addEventListener('did-navigate', sync as EventListener)
    webview.addEventListener('did-navigate-in-page', sync as EventListener)
    webview.addEventListener('did-fail-load', fail as EventListener)
    webview.addEventListener('dom-ready', ready)
    webview.addEventListener('page-title-updated', sync)
    if (readyRef.current === webview) {
      sync()
    }
    return () => {
      webview.removeEventListener('did-start-loading', start)
      webview.removeEventListener('did-start-navigation', navigate as EventListener)
      webview.removeEventListener('page-favicon-updated', favicon)
      webview.removeEventListener('did-stop-loading', stop)
      webview.removeEventListener('did-navigate', sync as EventListener)
      webview.removeEventListener('did-navigate-in-page', sync as EventListener)
      webview.removeEventListener('did-fail-load', fail as EventListener)
      webview.removeEventListener('dom-ready', ready)
      webview.removeEventListener('page-title-updated', sync)
    }
  }, [onTargetChange, webview])

  useEffect(() => {
    if (!webview || !resolvedSourceUrl) {
      return
    }
    const current = readyRef.current === webview ? webview.getURL() : webview.src
    if (current !== resolvedSourceUrl) {
      webview.src = resolvedSourceUrl
    }
  }, [resolvedSourceUrl, webview])

  const openDraft = () => {
    const next = normalizePreviewURL(draft)
    if (!next) {
      setError('Enter a website URL or an absolute file path.')
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
      readyRef.current = null
      setReadyWebview(null)
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
  const requestReload = useEffectEvent(reload)
  const handledReloads = useRef(reloads)
  useEffect(() => {
    if (reloads === handledReloads.current) return
    handledReloads.current = reloads
    requestReload()
  }, [reloads])

  const canAnnotate = canUseWebview && !!onAddBrowserAnnotation

  return (
    <SidePanelShell width={PREVIEW_PANEL_WIDTH} embedded={embedded} className={visible ? 'pointer-events-auto' : 'pointer-events-none'} onKeyDownCapture={find.handleKeyDownCapture}>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          openDraft()
        }}
        className="flex h-9 shrink-0 items-center gap-0.5 border-b border-border px-1.5 pointer-coarse:h-12"
      >
        <IconButton
          size="sm" className="pointer-coarse:size-10"
          aria-label="Back"
          title="Back"
          disabled={!webviewReady || !canGoBack}
          onClick={() => runWhenReady((view) => view.goBack())}
        >
          <ArrowLeft size={14} />
        </IconButton>
        <IconButton
          size="sm" className="pointer-coarse:size-10"
          aria-label="Forward"
          title="Forward"
          disabled={!webviewReady || !canGoForward}
          onClick={() => runWhenReady((view) => view.goForward())}
        >
          <ArrowRight size={14} />
        </IconButton>
        <IconButton
          size="sm" className="pointer-coarse:size-10"
          aria-label="Reload preview"
          title="Reload"
          disabled={!resolvedSourceUrl || (canUseWebview && !webviewReady)}
          onClick={reload}
        >
          {loading ? <LoaderCircle size={14} className="animate-spin" /> : <RotateCw size={14} />}
        </IconButton>
        <div className="flex min-w-0 flex-1 items-center gap-1.5 h-6 rounded-md bg-bg/60 px-2 pointer-coarse:h-8 ring-1 ring-border/70">
          <Globe size={13} className="shrink-0 text-ink-3" aria-hidden />
          <input
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="Website URL or file path"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent font-mono text-[12px] text-ink outline-none placeholder:text-ink-3"
          />
        </div>
        <IconButton
          aria-label={annotating ? 'Stop annotation' : 'Annotate preview'}
          title={annotating ? 'Stop annotation' : 'Annotate'}
          disabled={!resolvedSourceUrl || !webviewReady || !canAnnotate}
          onClick={() => (annotating ? void stopAnnotation() : void annotate())}
          size="sm"
          className={`pointer-coarse:size-10 ${annotating ? 'bg-primary/15 text-primary! hover:bg-primary/20' : 'text-ink-2'}`}
        >
          {annotating ? <SquareStop size={14} /> : <MessageCirclePlus size={15} />}
        </IconButton>
        <IconButton
          size="sm" className="pointer-coarse:size-10"
          aria-label="Open in Browser"
          title="Open in Browser"
          disabled={!resolvedSourceUrl}
          onClick={() => openExternal(resolvedSourceUrl)}
        >
          <ExternalLink size={14} />
        </IconButton>
        <BrowserMenu webContentsId={webviewReady && webview ? webview.getWebContentsId() : null} visible={visible} />
        {onClose ? <button
          type="button"
          aria-label="Hide side panel"
          onClick={onClose}
          className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-full pointer-coarse:size-10 text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
        >
          <X size={15} />
        </button> : null}
      </form>
      {error ? (
        <p className="shrink-0 border-b border-border px-3 py-2 text-[12px] text-danger">{error}</p>
      ) : null}
      <div className="relative min-h-0 flex-1 bg-bg">
        <div ref={cursorLayer} hidden={!visible} aria-hidden="true" className="pointer-events-none absolute inset-0 z-20 overflow-hidden" />
        <PreviewFindBar find={find} />
        {resolvedSourceUrl && canUseWebview ? (
          <webview
            ref={bindWebview}
            partition={PREVIEW_PARTITION}
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
