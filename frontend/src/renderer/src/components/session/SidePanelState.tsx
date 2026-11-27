import { useQuery } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState, type ReactNode } from 'react'
import { flushSync } from 'react-dom'
import { clientRuntime } from '@/lib/clientRuntime'
import { useBackendChange } from '@/lib/connection'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { isHTMLPath, resolveFileLink, type FileReference } from '@shared/fileReader'
import { entrypointKey, fileEntrypoint, mcpEntrypointsQuery } from '@/lib/api/mcp'
import { previewDisplayUrl } from '@/lib/api/preview'
import { openedFile } from '@/lib/mcpAppFiles'
import { useBrowserSessions, useSessionPreview } from '@/lib/browserSessions'
import { SidebarVisibility } from '@/lib/sidebar'
import type { PluginView } from '@/lib/api/types'
import type { MCPAppThread } from '@/lib/mcpAppThread'
import { OVERVIEW_PANEL_WIDTH, sidePanelTabs, pluginViewTab, refreshPluginTabs, type NewSidePanelTab, type PanelPresentation, type SidePanelMode, type SidePanelTab, type SidePanelTabs } from '@/lib/sidePanelTabs'

const PANEL_MIN_THREAD_WIDTH = 360

type SavedPanel = { state: SidePanelTabs; open: boolean; mode: SidePanelMode; widthOverride?: number }
const PanelStates = createContext<Map<string, SavedPanel> | null>(null)
export const SidePanelCloseGuard = createContext<((guard: () => Promise<boolean>) => () => void) | null>(null)

export function SidePanelStateProvider({ children }: { children: ReactNode }) {
  const [panels] = useState(() => new Map<string, SavedPanel>())
  useBackendChange(() => panels.clear())
  return <PanelStates.Provider value={panels}>{children}</PanelStates.Provider>
}

// A thread's panel starts closed unless its owner opens it with `startOpen`,
// then keeps its own state while the app runs. The agent's browser activity
// opens it only while `followBrowser` is set.
export function useSidePanelState(sessionId: string, { sideChatAvailable = false, canvasAvailable = true, startOpen = false, followBrowser = true, pluginViews }: {
  sideChatAvailable?: boolean
  canvasAvailable?: boolean
  startOpen?: boolean
  followBrowser?: boolean
  pluginViews?: PluginView[]
} = {}) {
  const panels = useContext(PanelStates)
  if (!panels) {
    throw new Error('Side panel state requires SidePanelStateProvider')
  }
  const saved = panels.get(sessionId)
  const initialPanel = useRef(saved)
  const restoredViews = useRef(false)
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
  const closeGuards = useRef(new Map<string, () => Promise<boolean>>())
  const tabs = useMemo(() => {
    const available = state.tabs.filter((tab) => (tab.kind !== 'side-chat' || sideChatAvailable) && (tab.kind !== 'canvas' || canvasAvailable))
    return pluginViews?.length ? refreshPluginTabs(available, pluginViews) : available
  }, [state.tabs, sideChatAvailable, canvasAvailable, pluginViews])
  const activeTab = tabs.find((tab) => tab.id === state.activeId) ?? tabs[0]
  const availableWidth = containerWidth - PANEL_MIN_THREAD_WIDTH
  const minWidth = Math.min(400, Math.max(240, availableWidth))
  const defaultWidth = Math.max(640, Math.round(containerWidth * 0.6))
  const maxWidth = Math.max(minWidth, availableWidth)
  const width = mode === 'overview' ? OVERVIEW_PANEL_WIDTH : clampWidth(widthOverride ?? defaultWidth, minWidth, maxWidth)
  const availableCSS = `calc(100% - ${PANEL_MIN_THREAD_WIDTH}px)`
  const preferredCSS = widthOverride === undefined ? 'max(640px, 60%)' : `${widthOverride}px`
  const widthStyle = mode === 'tabs'
    ? `clamp(min(400px, max(240px, ${availableCSS})), ${preferredCSS}, max(240px, ${availableCSS}))`
    : `${width}px`

  useLayoutEffect(() => {
    panels.set(sessionId, { state, open, mode, widthOverride })
  }, [panels, sessionId, state, open, mode, widthOverride])

  const showTabs = useCallback(() => {
    setSidebarOpen?.(false)
    setMode('tabs')
    setOpen(true)
  }, [setSidebarOpen])
  useEffect(() => {
    if (!pluginViews || restoredViews.current) return
    restoredViews.current = true
    if (!pluginViews.length) return
    dispatch({ type: 'restore', tabs: pluginViews.map(pluginViewTab) })
    if (!initialPanel.current) showTabs()
  }, [pluginViews, showTabs])
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
  useSessionPreview(sessionId, followBrowser ? showPreview : undefined)

  const addTab = useCallback((tab: NewSidePanelTab, after?: string) => {
    openTab(typeof tab === 'object' ? { id: `app:${entrypointKey(tab)}`, kind: 'app', app: tab }
      : tab === 'preview' ? { id: browsers.openTab(sessionId), kind: tab }
        : tab === 'file' ? { id: 'file', kind: tab, file: null }
          : { id: tab, kind: tab }, after)
  }, [browsers, openTab, sessionId])
  const openDiff = useCallback((repo: string) => openTab({ id: 'diff', kind: 'diff', repo }), [openTab])
  const duplicateTab = useCallback((id: string) => {
    openTab({ id: browsers.duplicate(sessionId, id), kind: 'preview' }, id)
  }, [browsers, openTab, sessionId])
  const registerCloseGuard = useCallback((id: string, guard: () => Promise<boolean>) => {
    closeGuards.current.set(id, guard)
    return () => { closeGuards.current.delete(id) }
  }, [])
  const closeTab = useCallback(async (id: string) => {
    const guard = closeGuards.current.get(id)
    if (guard && !await guard()) return
    browsers.close(id)
    dispatch({ type: 'close', id })
  }, [browsers])
  const showFile = useCallback((ref: FileReference) => {
    // A connected server's file viewer opens its files in place of the default one.
    const viewer = fileEntrypoint(entrypoints ?? [], ref.path)
    if (viewer) {
      openTab({ id: `app:${entrypointKey(viewer)}:${ref.path}`, kind: 'app', app: viewer, file: openedFile(sessionId, ref.path) })
    } else if (isHTMLPath(ref.path)) {
      openTab({ id: browsers.openTab(sessionId, ref.path), kind: 'preview' })
    } else {
      openTab({ id: `file:${ref.path}`, kind: 'file', file: { ...ref, location: ref.location && { ...ref.location } } })
    }
  }, [browsers, entrypoints, openTab, sessionId])
  const openFile = useCallback((file: string | FileReference) => {
    const ref = typeof file === 'string' ? resolveFileLink(file) : file
    if (!ref) return false
    showFile(ref)
    return true
  }, [showFile])
  // An inline app view opens larger as a tab showing the same call.
  const openApp = useCallback<NonNullable<MCPAppThread['expand']>>((view, app, call) => {
    const points = (entrypoints ?? []).filter((point) => point.server_id === app.server_id)
    openTab({
      id: `app:${view}`,
      kind: 'app',
      app: { ...app, title: points.find((point) => point.tool === app.tool)?.title ?? app.tool, icon: points[0]?.icon },
      call,
    })
  }, [entrypoints, openTab])
  const openView = useCallback((presentation: PanelPresentation) => {
    if (presentation.kind === 'canvas') {
      if (canvasAvailable) openTab({ id: 'canvas', kind: 'canvas' })
    } else {
      openTab(pluginViewTab(presentation.view))
    }
  }, [openTab, canvasAvailable])
  const openPreview = useCallback((url: string) => {
    const display = previewDisplayUrl(url) ?? url
    if (openFile(display)) {
      return
    }
    openTab({ id: browsers.openTab(sessionId, url), kind: 'preview' })
  }, [browsers, openFile, openTab, sessionId])
  useEffect(() => clientRuntime.onOpenPreviewURL?.(openPreview), [openPreview])

  const closeBrowserTab = useCallback(() => {
    if (!open || mode !== 'tabs' || modalDialogOpen()) return false
    if (activeTab?.kind === 'preview') closeTab(activeTab.id)
    else if (!activeTab) close()
    else return false
    return true
  }, [open, mode, activeTab, closeTab, close])
  useEffect(() => clientRuntime.onBrowserCloseShortcut?.(closeBrowserTab), [closeBrowserTab])

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
    if (key === 'w' && !event.repeat && closeBrowserTab()) {
      event.preventDefault()
      return
    }
    if (key === 'o') {
      event.preventDefault()
      toggleMode('overview')
      return
    }
    const shortcuts = { j: 'side-chat', d: 'diff', p: 'preview', t: 'terminal' } as const
    const kind = shortcuts[key as keyof typeof shortcuts]
    if (!kind || kind === 'side-chat' && !sideChatAvailable) {
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
    toggleMode, close, selectTab, reorderTabs, addTab, openDiff, duplicateTab, closeTab, registerCloseGuard, openFile, openPreview, openApp, openView,
  }
}

function clampWidth(width: number, minWidth: number, maxWidth: number): number {
  return Math.round(Math.min(Math.max(width, minWidth), maxWidth))
}
