import { AlertTriangle, LoaderCircle } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import {
  appContentFileUrl,
  callAppTool,
  completeAppAI,
  deleteAppKV,
  fetchAppContentText,
  getAppKV,
  issueAppTicket,
  listAppKV,
  putAppKV,
  type AppTicketResponse,
  type JazApp,
} from '@/lib/api/apps'
import {
  buildAppSandboxDocument,
  dispatchAppBridgeCall,
  isAppBridgeRequest,
  type AppBridgeAPI,
} from '@/lib/appBridge'

const APP_BRIDGE_API: AppBridgeAPI = {
  callAppTool,
  completeAppAI,
  deleteAppKV,
  getAppKV,
  listAppKV,
  putAppKV,
}

function isSafeExternalLink(href: string): boolean {
  try {
    const parsed = new URL(href)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}
import { artifactErrorForDisplay } from '@/lib/artifacts'
import { clientRuntime, openExternal } from '@/lib/clientRuntime'
import { useArtifactThemeCSS } from '@/lib/hooks/useArtifactThemeCSS'
import type { DesktopAppPermission } from '@shared/desktopApps'

const SHARED_WRITE_ACK_PREFIX = 'jaz.app.sharedWriteAck.'

function hasSharedWriteAck(appId: string): boolean {
  try {
    return localStorage.getItem(SHARED_WRITE_ACK_PREFIX + appId) === '1'
  } catch {
    return false
  }
}

function recordSharedWriteAck(appId: string): void {
  try {
    localStorage.setItem(SHARED_WRITE_ACK_PREFIX + appId, '1')
  } catch {
    // ignore storage failures
  }
}

export function AppRenderer({
  app,
  version,
  fillHeight = true,
  desktopPermissions,
  desktopPermissionsPaused = false,
  className = '',
  onReady,
}: {
  app: JazApp
  version?: number
  fillHeight?: boolean
  desktopPermissions?: DesktopAppPermission[]
  desktopPermissionsPaused?: boolean
  className?: string
  // Fires once the app has something to show, and again when its frame reloads.
  onReady?: () => void
}) {
  const themeCSS = useArtifactThemeCSS()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const [ticket, setTicket] = useState<AppTicketResponse | null>(null)
  const [content, setContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [runtimeError, setRuntimeError] = useState('')
  const [height, setHeight] = useState(420)
  const [sharedPromptOpen, setSharedPromptOpen] = useState(false)
  const sharedPromptResolverRef = useRef<((allowed: boolean) => void) | null>(null)
  const sharedPromptPromiseRef = useRef<Promise<boolean> | null>(null)

  const targetVersion = version && version > 0 ? version : app.current_version

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    setRuntimeError('')
    setContent(null)
    void issueAppTicket(app.id, targetVersion)
      .then(async (issued) => {
        if (controller.signal.aborted) return
        setTicket(issued)
        const text = await fetchAppContentText(issued, issued.entry_path, controller.signal)
        if (controller.signal.aborted) return
        setContent(text)
        setLoading(false)
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        setError(err instanceof Error ? err.message : 'Could not load app content')
        setLoading(false)
      })
    return () => controller.abort()
  }, [app.id, targetVersion, app.bundle_sha256])

  const confirmSharedWrite = useCallback(async (): Promise<boolean> => {
    if (hasSharedWriteAck(app.id)) return true
    if (sharedPromptPromiseRef.current) return sharedPromptPromiseRef.current
    const promise = new Promise<boolean>((resolve) => {
      sharedPromptResolverRef.current = resolve
      setSharedPromptOpen(true)
    })
    sharedPromptPromiseRef.current = promise
    return promise
  }, [app.id])

  const resolveSharedPrompt = (allowed: boolean) => {
    setSharedPromptOpen(false)
    if (allowed) recordSharedWriteAck(app.id)
    const resolver = sharedPromptResolverRef.current
    sharedPromptResolverRef.current = null
    sharedPromptPromiseRef.current = null
    resolver?.(allowed)
  }

  const isMarkdownEntry =
    app.kind === 'document' && /\.(md|markdown)$/i.test(ticket?.entry_path ?? app.entry_path ?? '')

  const sandboxDoc = useMemo(() => {
    if (isMarkdownEntry || content === null) return ''
    return buildAppSandboxDocument(
      app.title,
      content,
      themeCSS,
      app.capabilities ?? [],
      app.allowed_tools ?? [],
      { measureLayout: fillHeight },
    )
  }, [app.allowed_tools, app.capabilities, app.title, content, fillHeight, isMarkdownEntry, themeCSS])

  useEffect(() => {
    if (!ticket || isMarkdownEntry) return
    const onMessage = (event: MessageEvent) => {
      const win = iframeRef.current?.contentWindow
      if (!win || event.source !== win) return
      const data = event.data as Record<string, unknown> | null
      if (!data || typeof data !== 'object') return

      if (data.type === 'jaz:artifact-height' && typeof data.height === 'number' && !fillHeight) {
        if (Number.isFinite(data.height)) setHeight(Math.max(200, Math.ceil(data.height)))
        return
      }
      if (data.type === 'jaz:artifact-error' && typeof data.message === 'string') {
        const display = artifactErrorForDisplay(data.message)
        if (display) setRuntimeError(display)
        return
      }
      if (data.type === 'jaz:artifact-link' && typeof data.href === 'string' && isSafeExternalLink(data.href)) {
        openExternal(data.href)
        return
      }
      if (isAppBridgeRequest(data)) {
        const reqId = data.id
        void dispatchAppBridgeCall(
          {
            app,
            ticket,
            api: APP_BRIDGE_API,
            resolveBundleAssetUrl: (relpath) => appContentFileUrl(ticket, relpath),
            confirmSharedWrite,
            desktopApps: clientRuntime.desktopApps,
            desktopPermissions,
            desktopPermissionsPaused,
          },
          data.method,
          data.params ?? {},
        ).then(
          (result) => {
            if (iframeRef.current?.contentWindow === win) {
              win.postMessage({ type: 'jaz:app-bridge-response', id: reqId, result }, '*')
            }
          },
          (err: unknown) => {
            if (iframeRef.current?.contentWindow === win) {
              win.postMessage(
                {
                  type: 'jaz:app-bridge-response',
                  id: reqId,
                  error: err instanceof Error ? err.message : 'App bridge call failed',
                },
                '*',
              )
            }
          },
        )
      }
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [app, confirmSharedWrite, desktopPermissions, desktopPermissionsPaused, fillHeight, isMarkdownEntry, ticket])

  const shownWithoutFrame = !loading && (isMarkdownEntry || Boolean(error))
  useEffect(() => {
    if (shownWithoutFrame) onReady?.()
  }, [onReady, shownWithoutFrame])

  if (loading) {
    return (
      <div className={`flex items-center justify-center gap-2 p-6 text-[13px] text-ink-3 ${className}`}>
        <LoaderCircle size={14} className="animate-spin" aria-hidden />
        Loading app…
      </div>
    )
  }

  if (error) {
    return (
      <div className={`rounded-card border border-border bg-danger-soft/60 p-4 text-[13px] text-danger ${className}`}>
        {error}
      </div>
    )
  }

  return (
    <div className={`relative flex flex-col ${fillHeight ? 'h-full min-h-0 flex-1' : 'w-full'} ${className}`}>
      {runtimeError ? (
        <div className="flex shrink-0 items-center justify-between gap-2 border-b border-border bg-danger-soft px-3 py-1.5 text-[12px] text-danger">
          <span className="truncate font-mono">{runtimeError}</span>
          <button
            type="button"
            onClick={() => setRuntimeError('')}
            className="shrink-0 text-[11px] underline"
          >
            Dismiss
          </button>
        </div>
      ) : null}

      {isMarkdownEntry ? (
        <div className="scrollbar-quiet min-h-0 flex-1 overflow-auto bg-bg px-6 py-5">
          <div className="mx-auto max-w-[780px]">
            <RenderedMarkdown text={content ?? ''} className="file-prose" />
          </div>
        </div>
      ) : (
        <iframe
          ref={iframeRef}
          title={app.title}
          sandbox="allow-scripts allow-forms"
          referrerPolicy="no-referrer"
          srcDoc={sandboxDoc}
          onLoad={onReady}
          style={fillHeight ? undefined : { height }}
          className={`block w-full border-0 bg-transparent ${fillHeight ? 'h-full min-h-0 flex-1' : ''}`}
        />
      )}

      <Modal
        open={sharedPromptOpen}
        onClose={() => resolveSharedPrompt(false)}
        title="Write to shared app storage?"
        description={`"${app.title}" wants to save data to shared storage. Anything written here is visible to everyone who has access to this app.`}
        size="sm"
        footer={
          <>
            <div />
            <div className="flex items-center gap-1.5">
              <Button variant="ghost" size="sm" onClick={() => resolveSharedPrompt(false)}>
                Cancel
              </Button>
              <Button variant="primary" size="sm" onClick={() => resolveSharedPrompt(true)}>
                Allow shared writes
              </Button>
            </div>
          </>
        }
      >
        <div className="flex items-start gap-2.5 rounded-control border border-border bg-surface p-3 text-[12.5px] text-ink-2">
          <AlertTriangle size={15} className="mt-0.5 shrink-0 text-amber-500" aria-hidden />
          <p>
            Avoid storing secrets or private personal notes in shared storage. Personal storage remains private to your account.
          </p>
        </div>
      </Modal>
    </div>
  )
}
