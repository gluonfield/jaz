import { useCallback, useLayoutEffect, useRef, useState, useSyncExternalStore, type CSSProperties, type ReactNode } from 'react'
import { PreviewPanel, PREVIEW_PANEL_WIDTH } from '@/components/session/PreviewPanel'
import { BrowserSessions, BrowserSessionsContext, type BrowserPresentation, type BrowserSession, type PreviewTarget, useBrowserSessions } from '@/lib/browserSessions'
import { clientRuntime } from '@/lib/clientRuntime'
import { useBackendChange } from '@/lib/connection'
import { useSideBrowser } from '@/lib/hooks/useSideBrowser'
import { BROWSER_IDLE_MS } from '@/lib/browserRetention'
import { useBrowserHost } from '@/lib/hooks/useBrowserHost'

export function BrowserWorkspace({ children, idleMs = BROWSER_IDLE_MS }: { children: ReactNode; idleMs?: number }) {
  const [sessions] = useState(() => new BrowserSessions())
  useBrowserHost(sessions)
  const entries = useSyncExternalStore(sessions.subscribe, sessions.getSnapshot)
  useBackendChange(() => sessions.clear())
  return (
    <BrowserSessionsContext.Provider value={sessions}>
      {children}
      {clientRuntime.capabilities.previewWebview && entries.map((entry) => (
        <BrowserSessionPanel key={`${entry.id}:${entry.generation ?? 0}`} entry={entry} idleMs={idleMs} />
      ))}
    </BrowserSessionsContext.Provider>
  )
}

function BrowserSessionPanel({ entry, idleMs }: { entry: BrowserSession; idleMs: number }) {
  const sessions = useBrowserSessions()
  const open = useCallback((url: string) => sessions.open(entry.id, url), [sessions, entry.id])
  const show = useCallback(() => sessions.show(entry.id), [sessions, entry.id])
  const update = useCallback((target: PreviewTarget) => sessions.update(entry.id, { target }), [sessions, entry.id])
  const visible = Boolean(entry.presentation)
  const { browser, resident } = useSideBrowser({ sessionId: entry.id, open, show, visible, url: entry.target.sourceUrl, idleMs, controlled: !entry.ownerId || entry.ownerId === entry.id })
  const host = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ width: PREVIEW_PANEL_WIDTH, height: Math.max(400, window.innerHeight - 52) })
  useLayoutEffect(() => {
    if (!visible || !host.current) return
    const observer = new ResizeObserver(([observation]) => {
      const { width, height } = observation.contentRect
      if (width && height) setSize({ width, height })
    })
    observer.observe(host.current)
    return () => observer.disconnect()
  }, [visible])
  return (
    <div
      ref={host}
      data-browser-session={entry.id}
      className="max-sm:z-shell"
      inert={!visible}
      aria-hidden={!visible}
      style={{
        position: 'fixed',
        positionAnchor: '--jaz-browser-panel',
        top: visible ? 'anchor(top)' : 52,
        left: visible ? 'anchor(left)' : 0,
        width: visible ? 'anchor-size(width)' : size.width,
        height: visible ? 'anchor-size(height)' : size.height,
        opacity: visible ? 1 : 0,
        pointerEvents: 'none',
        overflow: 'hidden',
        '--side-panel-width': '100%',
      } as CSSProperties}
    >
      {resident && <PreviewPanel
        browserControl={browser}
        target={entry.target}
        reloads={entry.reloads}
        onTargetChange={update}
        visible={visible}
        onClose={entry.presentation?.onClose}
        embedded={entry.embedded}
        onAddBrowserAnnotation={entry.presentation?.onAddBrowserAnnotation}
        onUploadAttachment={entry.presentation?.onUploadAttachment}
      />}
    </div>
  )
}

export function BrowserPanelSlot({ sessionId, visible, ...presentation }: BrowserPresentation & { sessionId: string; visible: boolean }) {
  const sessions = useBrowserSessions()
  const { onClose, onAddBrowserAnnotation, onUploadAttachment, embedded } = presentation
  const getEntry = useCallback(() => sessions.getSnapshot().find((entry) => entry.id === sessionId), [sessions, sessionId])
  const entry = useSyncExternalStore(sessions.subscribe, getEntry)
  const update = useCallback((target: PreviewTarget) => sessions.update(sessionId, { target }), [sessions, sessionId])
  useLayoutEffect(() => {
    if (!visible) return
    return sessions.present(sessionId, { onClose, onAddBrowserAnnotation, onUploadAttachment, embedded })
  }, [sessions, sessionId, visible, onClose, onAddBrowserAnnotation, onUploadAttachment, embedded])
  if (!clientRuntime.capabilities.previewWebview) {
    return entry ? <PreviewPanel {...presentation} visible={visible} target={entry.target} reloads={entry.reloads} onTargetChange={update} /> : null
  }
  return <div className="h-full max-sm:w-full!" style={{ anchorName: '--jaz-browser-panel', width: `var(--side-panel-width, ${PREVIEW_PANEL_WIDTH}px)` }} />
}
