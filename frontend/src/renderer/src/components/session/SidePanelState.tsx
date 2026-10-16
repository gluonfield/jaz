import { useQuery } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState, type ReactNode } from 'react'
import { flushSync } from 'react-dom'
import { clientRuntime } from '@/lib/clientRuntime'
import { useBackendChange } from '@/lib/connection'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { isHTMLPath, parseFileReference, type FileReference } from '@shared/fileReader'
import { entrypointKey, fileEntrypoint, mcpEntrypointsQuery } from '@/lib/api/mcp'
import { previewDisplayUrl } from '@/lib/api/preview'
import { openedFile } from '@/lib/mcpAppFiles'
import { useBrowserSessions, useSessionPreview } from '@/lib/browserSessions'
import { SidebarVisibility } from '@/lib/sidebar'
import { OVERVIEW_PANEL_WIDTH, sidePanelTabs, type NewSidePanelTab, type SidePanelMode, type SidePanelTab, type SidePanelTabs } from '@/lib/sidePanelTabs'

const PANEL_MAX_WIDTH = 1180
const PANEL_MIN_THREAD_WIDTH = 360

type SavedPanel = { state: SidePanelTabs; open: boolean; mode: SidePanelMode; widthOverride?: number }
const PanelStates = createContext<Map<string, SavedPanel> | null>(null)

export function SidePanelStateProvider({ children }: { children: ReactNode }) {
  const [panels] = useState(() => new Map<string, SavedPanel>())
  useBackendChange(() => panels.clear())
  return <PanelStates.Provider value={panels}>{children}</PanelStates.Provider>
}

// A thread's panel starts closed unless its owner opens it with `startOpen`,
// then keeps its own state while the app runs.
export function useSidePanelState(sessionId: string, sideChatAvailable = false, startOpen = false) {
  const panels = useContext(PanelStates)
  if (!panels) {
    throw new Error('Side panel state requires SidePanelStateProvider')
  }
  const saved = panels.get(sessionId)
  const setSidebarOpen = useContext(SidebarVisibility)
  const browsers = useBrowserSessions()
  const entrypoints = useQuery(mcpEntrypointsQuery).data
  const [state, dispatch] = useReducer(sidePanelTabs, undefined, () => {
    if (saved) {
      return saved.state
    }
    const tabs: SidePanelTab[] = browsers.getSnapshot()
      .filter((entry) => entry.ownerId === sessionId)
      .map((entry) => ({ id: entry.id, kind: 'preview' }))
    return { tabs, activeId: tabs[0]?.id ?? null }
  })
  const [containerWidth, setContainerWidth] = useState(0)
  const container = useRef<HTMLDivElement | null>(null)
  const containerRef = useCallback((element: HTMLDivElement | null) => {
    container.current = element
    if (!element) {
      return
    }
    const observer = new ResizeObserver(([entry]) => setContainerWidth(Math.round(entry.contentRect.width)))
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  const [open, setOpen] = useState(() => saved?.open ?? startOpen)
  const [mode, setMode] = useState<SidePanelMode>(saved?.mode ?? 'overview')
  const [widthOverride, setWidthOverride] = useState(saved?.widthOverride)
  const [resizing, setResizing] = useState(false)
  const tabs = useMemo(() => state.tabs.filter((tab) => tab.kind !== 'side-chat' || sideChatAvailable), [state.tabs, sideChatAvailable])
  const activeTab = tabs.find((tab) => tab.id === state.activeId) ?? tabs[0]
  const availableWidth = containerWidth - PANEL_MIN_THREAD_WIDTH
  const minWidth = Math.min(400, Math.max(240, availableWidth))
  const defaultWidth = Math.max(640, Math.round(containerWidth * 0.6))
  const maxWidth = Math.max(minWidth, Math.min(PANEL_MAX_WIDTH, availableWidth))
  const width = mode === 'overview' ? OVERVIEW_PANEL_WIDTH : clampWidth(widthOverride ?? defaultWidth, minWidth, maxWidth)
  const availableCSS = `calc(100% - ${PANEL_MIN_THREAD_WIDTH}px)`
  const preferredCSS = widthOverride === undefined ? 'max(640px, 60%)' : `${widthOverride}px`
  const widthStyle = mode === 'tabs'
    ? `clamp(min(400px, max(240px, ${availableCSS})), ${preferredCSS}, max(240px, min(${PANEL_MAX_WIDTH}px, ${availableCSS})))`
    : `${width}px`

  useLayoutEffect(() => {
    panels.set(sessionId, { state, open, mode, widthOverride })
  }, [panels, sessionId, state, open, mode, widthOverride])

  const showTabs = useCallback(() => {
    setSidebarOpen?.(false)
    setMode('tabs')
    setOpen(true)
  }, [setSidebarOpen])
  const toggleMode = useCallback((next: SidePanelMode) => {
    if (open && mode === next) {
      setOpen(false)
    } else if (next === 'tabs') {
      showTabs()
    } else {
      setMode(next)
      setOpen(true)
    }
  }, [open, mode, showTabs])
  const close = useCallback(() => setOpen(false), [])
  const resize = useCallback((next: number) => setWidthOverride(clampWidth(next, minWidth, maxWidth)), [minWidth, maxWidth])
  const selectTab = useCallback((id: string) => {
    dispatch({ type: 'select', id })
    showTabs()
  }, [showTabs])
  const reorderTabs = useCallback((ids: string[]) => dispatch({ type: 'reorder', ids }), [])
  const openTab = useCallback((tab: SidePanelTab, after?: string) => {
    dispatch({ type: 'open', tab, after })
    showTabs()
  }, [showTabs])
  const showPreview = useCallback(async () => {
    flushSync(() => openTab({ id: sessionId, kind: 'preview' }))
    await Promise.allSettled(container.current?.getAnimations({ subtree: true })
      .filter((animation) => animation instanceof CSSTransition && animation.transitionProperty === 'width')
      .map((animation) => animation.finished) ?? [])
  }, [openTab, sessionId])
  useSessionPreview(sessionId, showPreview)

  const addTab = useCallback((tab: NewSidePanelTab, after?: string) => {
    openTab(typeof tab === 'object' ? { id: `app:${entrypointKey(tab)}`, kind: 'app', app: tab }
      : tab === 'preview' ? { id: browsers.openTab(sessionId), kind: tab }
        : tab === 'file' ? { id: 'file', kind: tab, file: null }
          : { id: tab, kind: tab }, after)
  }, [browsers, openTab, sessionId])
  const duplicateTab = useCallback((id: string) => {
    openTab({ id: browsers.duplicate(sessionId, id), kind: 'preview' }, id)
  }, [browsers, openTab, sessionId])
  const closeTab = useCallback((id: string) => {
    browsers.close(id)
    dispatch({ type: 'close', id })
  }, [browsers])
  const openFile = useCallback((file: string | FileReference) => {
    const ref = typeof file === 'string' ? parseFileReference(file) : file
    if (!ref) {
      return false
    }
    // A connected server's file viewer opens its files in place of the default one.
    const viewer = fileEntrypoint(entrypoints ?? [], ref.path)
    if (viewer) {
      openTab({ id: `app:${entrypointKey(viewer)}:${ref.path}`, kind: 'app', app: viewer, file: openedFile(sessionId, ref.path) })
    } else if (isHTMLPath(ref.path)) {
      openTab({ id: browsers.openTab(sessionId, ref.path), kind: 'preview' })
    } else {
      openTab({ id: `file:${ref.path}`, kind: 'file', file: ref })
    }
    return true
  }, [browsers, entrypoints, openTab, sessionId])
  const openPreview = useCallback((url: string) => {
    const display = previewDisplayUrl(url) ?? url
    if (openFile(display)) {
      return
    }
    openTab({ id: browsers.openTab(sessionId, url), kind: 'preview' })
  }, [browsers, openFile, openTab, sessionId])
  useEffect(() => clientRuntime.onOpenPreviewURL?.(openPreview), [openPreview])

  useWindowEvent('keydown', (event) => {
    if (!(event.metaKey || event.ctrlKey) || event.defaultPrevented || event.altKey || modalDialogOpen()) {
      return
    }
    const key = event.key.toLowerCase()
    if (event.shiftKey) {
      if (key === 's') {
        event.preventDefault()
        if (open) {
          close()
        } else {
          showTabs()
        }
      }
      return
    }
    if (!event.metaKey || event.ctrlKey) {
      return
    }
    if (key === 'o') {
      event.preventDefault()
      toggleMode('overview')
      return
    }
    const shortcuts = { j: 'side-chat', d: 'diff', p: 'preview', t: 'terminal' } as const
    const kind = shortcuts[key as keyof typeof shortcuts]
    if (!kind || (kind === 'side-chat' && !sideChatAvailable)) {
      return
    }
    event.preventDefault()
    const existing = tabs.find((tab) => tab.kind === kind)
    if (existing) {
      selectTab(existing.id)
    } else {
      addTab(kind)
    }
  })

  return {
    containerRef, open, mode, tabs, activeTab,
    resize, resizing, setResizing, width, widthStyle, minWidth, maxWidth,
    resizable: mode === 'tabs',
    toggleMode, close, selectTab, reorderTabs, addTab, duplicateTab, closeTab, openFile, openPreview,
  }
}

function clampWidth(width: number, minWidth: number, maxWidth: number): number {
  return Math.round(Math.min(Math.max(width, minWidth), maxWidth))
}
