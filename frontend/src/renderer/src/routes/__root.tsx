import {
  Outlet,
  createRootRoute,
  useNavigate,
  useRouter,
  useRouterState,
} from '@tanstack/react-router'
import { motion } from 'motion/react'
import {
  type PointerEvent as ReactPointerEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { ConnectOverlay } from '@/components/connection/ConnectOverlay'
import { CommandPalette } from '@/components/search/CommandPalette'
import { isSettingsSection, type SettingsSection } from '@/components/settings/sections'
import { MCPApps } from '@/components/apps/MCPAppFrame'
import { SettingsOverlay } from '@/components/settings/SettingsOverlay'
import { NavRail, railTab } from '@/components/sidebar/NavRail'
import { BotsPanel } from '@/components/bots/BotsPanel'
import { ChatPanel } from '@/components/sidebar/ChatPanel'
import { Sidebar } from '@/components/sidebar/Sidebar'
import { TitlebarNavigation } from '@/components/sidebar/TitlebarNavigation'
import { ToastProvider } from '@/components/ui/toast'
import { clientRuntime, isMacDesktop } from '@/lib/clientRuntime'
import { SidebarVisibility } from '@/lib/sidebar'
import { SidePanelSlot } from '@/lib/sidePanelSlot'
import { VoiceProvider } from '@/lib/voice/VoiceProvider'
import { GlobalVoice } from '@/components/session/FloatingVoice'
import { VoiceDesktopBridge } from '@/lib/voice/VoiceDesktopBridge'
import { drawerSlide } from '@/lib/dom/drawer'
import { modalDialogOpen } from '@/lib/dom/modal'
import { isMobileViewport, useIsMobile } from '@/lib/hooks/useIsMobile'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { TitlebarActionsOutlet, TitlebarProvider, TitlebarSlotOutlet } from '@/lib/titlebar'
import type { BrowserNavigationDirection } from '../../../shared/browserNavigation'

type RootSearch = { settings?: SettingsSection }
type SettingsSectionNavigation = { replace?: boolean }

export const Route = createRootRoute({
  // Settings rides in the URL so it's a real history entry that ⌘[ / ⌘] and the
  // browser back/forward step in and out of like any other page.
  validateSearch: (search): RootSearch =>
    isSettingsSection(search.settings) ? { settings: search.settings } : {},
  component: RootComponent,
})

// Board windows render their route full-bleed: no sidebar, no app titlebar
// (they use the native OS titlebar). windowKind is fixed per window, so the
// branch never changes within a window's lifetime.
function RootComponent() {
  if (clientRuntime.windowKind === 'board') {
    return <BoardRoot />
  }
  if (clientRuntime.windowKind === 'launcher') {
    return <LauncherRoot />
  }
  return <VoiceProvider><VoiceDesktopBridge /><RootLayout /></VoiceProvider>
}

function LauncherRoot() {
  return (
    <ToastProvider>
      <main className="h-full overflow-hidden bg-transparent">
        <Outlet />
      </main>
    </ToastProvider>
  )
}

function BoardRoot() {
  const handleBrowserNavigation = useCallback((direction: BrowserNavigationDirection) => {
    if (direction === 'back') window.history.back()
    else window.history.forward()
  }, [])

  useEffect(
    () => clientRuntime.onBrowserNavigation?.(handleBrowserNavigation),
    [handleBrowserNavigation],
  )

  // No extra padding: the board page is h-full, so any would overflow into
  // a permanent sliver of scrollbar.
  return (
    <ToastProvider>
      <main className="h-full overflow-hidden bg-bg">
        <Outlet />
      </main>
    </ToastProvider>
  )
}

const SIDEBAR_DEFAULT_WIDTH = 264
const SIDEBAR_MIN_WIDTH = 200
const SIDEBAR_MAX_WIDTH = 480
const SIDEBAR_PREF_KEY = 'jaz.sidebar'
const SIDEBAR_WIDTH_KEY = 'jaz.sidebarWidth'

const clampSidebarWidth = (w: number) =>
  Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(w)))

function RootLayout() {
  const navigate = useNavigate()
  const router = useRouter()

  // Deep links from board windows ("Open bot in Jaz") land here.
  useEffect(() => {
    return clientRuntime.onOpenRoute?.((path) => router.history.push(path))
  }, [router])
  // Phones start with the full-screen drawer closed so the thread shows first;
  // the stored preference only governs the desktop column.
  const [sidebarOpen, setSidebarOpen] = useState(
    () => !isMobileViewport() && localStorage.getItem(SIDEBAR_PREF_KEY) !== 'closed',
  )
  const [sidebarWidth, setSidebarWidth] = useState(() => {
    const stored = Number(localStorage.getItem(SIDEBAR_WIDTH_KEY))
    return stored > 0 ? clampSidebarWidth(stored) : SIDEBAR_DEFAULT_WIDTH
  })
  const [resizing, setResizing] = useState(false)
  const [connectOpen, setConnectOpen] = useState(false)
  const [commandOpen, setCommandOpen] = useState(false)
  const [panelRow, setPanelRow] = useState<HTMLDivElement | null>(null)
  const [panelSlot, setPanelSlot] = useState<HTMLDivElement | null>(null)
  const sidePanelSlot = useMemo(() => ({ slot: panelSlot, row: panelRow }), [panelSlot, panelRow])

  // Settings open-state is derived from the URL, not local state. lastSection
  // lets the sidebar button reopen the pane the user last viewed.
  const settingsSection = Route.useSearch().settings
  const settingsOpen = settingsSection !== undefined
  const lastSection = useRef<SettingsSection>('general')
  useEffect(() => {
    if (settingsSection) lastSection.current = settingsSection
  }, [settingsSection])

  const openSettings = useCallback(
    (section?: SettingsSection) =>
      void navigate({
        to: '.',
        search: (prev) => ({ ...prev, settings: section ?? lastSection.current }),
      }),
    [navigate],
  )
  const goToSettingsSection = useCallback(
    (section: SettingsSection, options?: SettingsSectionNavigation) => {
      if (section === settingsSection) return
      void navigate({
        to: '.',
        search: (prev) => ({ ...prev, settings: section }),
        replace: options?.replace,
      })
    },
    [navigate, settingsSection],
  )
  const closeSettings = useCallback(
    () => void navigate({ to: '.', search: (prev) => ({ ...prev, settings: undefined }) }),
    [navigate],
  )

  // Phone: the sidebar is a full-screen drawer (CSS `max-sm:w-full`) that slides
  // over the thread rather than a resizable column, and auto-dismisses on
  // navigation to reveal the thread underneath. On desktop the thread panel
  // belongs to Chat and Bots; the other sections take the full card.
  // Only the toggle animates the panel: switching tabs hides and restores it
  // instantly.
  const isMobile = useIsMobile()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const tab = railTab(pathname, settingsOpen)
  const hasPanel = isMobile || tab === 'chat' || tab === '/bots'
  const panelOpen = hasPanel && sidebarOpen
  // A specific board paints itself on bg-surface so its tiles blend; match main
  // to surface there too so no bg-bg seam shows around the board.
  const onBoard = /^\/boards\/.+/.test(pathname)
  useEffect(() => {
    if (isMobile) setSidebarOpen(false)
  }, [isMobile, pathname])

  useEffect(() => {
    // The phone drawer's open/closed state is transient; only persist the
    // desktop column preference so a narrow viewport never clobbers it.
    if (isMobile) return
    localStorage.setItem(SIDEBAR_PREF_KEY, sidebarOpen ? 'open' : 'closed')
  }, [isMobile, sidebarOpen])

  useEffect(() => {
    localStorage.setItem(SIDEBAR_WIDTH_KEY, String(sidebarWidth))
  }, [sidebarWidth])

  const handleBrowserNavigation = useCallback(
    (direction: BrowserNavigationDirection) => {
      if (commandOpen) {
        if (direction === 'back') setCommandOpen(false)
        return
      }
      // Settings is a normal history entry now, so it must not trip the modal
      // guard — let back/forward step out of and into it like any page.
      if (!settingsOpen && modalDialogOpen()) return
      if (direction === 'back') window.history.back()
      else window.history.forward()
    },
    [commandOpen, settingsOpen],
  )

  const startResize = (e: ReactPointerEvent) => {
    e.preventDefault()
    const startX = e.clientX
    const startWidth = sidebarWidth
    setResizing(true)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'

    const onMove = (ev: PointerEvent) => {
      setSidebarWidth(clampSidebarWidth(startWidth + ev.clientX - startX))
    }
    const onUp = () => {
      setResizing(false)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  useEffect(
    () => clientRuntime.onBrowserNavigation?.(handleBrowserNavigation),
    [handleBrowserNavigation],
  )

  // Cmd+S toggles the sidebar where the tab has one — unless something closer
  // (the agent-file editor's save keymap) already claimed the event. Cmd+N starts a thread.
  // Cmd+K toggles the command palette. Cmd+, opens Settings. Cmd+[ / Cmd+]
  // follow browser history.
  useWindowEvent('keydown', (e) => {
    if (e.defaultPrevented) return
    const navigation = browserNavigationDirection(e)
    if (navigation) {
      e.preventDefault()
      handleBrowserNavigation(navigation)
      return
    }
    if (!(e.metaKey || e.ctrlKey)) return
    const key = e.key.toLowerCase()
    if (!e.shiftKey && !e.altKey && key === ',') {
      e.preventDefault()
      setCommandOpen(false)
      setConnectOpen(false)
      openSettings()
      return
    }
    if (!e.shiftKey && key === 's' && hasPanel) {
      e.preventDefault()
      setSidebarOpen((open) => !open)
    }
    if (key === 'n') {
      e.preventDefault()
      navigate({ to: '/new' })
    }
    if (key === 'k') {
      // Settings is a rail section, so ⌘K works there unless one of its own
      // dialogs is open on top.
      if (!commandOpen && modalDialogOpen(settingsOpen ? 1 : 0)) return
      e.preventDefault()
      setCommandOpen((open) => !open)
    }
  })

  const slide = resizing ? { duration: 0 } : { duration: 0.2, ease: [0.4, 0, 0.2, 1] as const }
  // On desktop there's no titlebar strip: the window buttons sit above the
  // thread list and hide while the list is collapsed, so they never jump.
  const windowButtonsVisible = isMobile || panelOpen
  useEffect(() => {
    clientRuntime.setWindowButtonsVisible?.(windowButtonsVisible)
  }, [windowButtonsVisible])

  const navigation = (
    <TitlebarNavigation
      hasPanel={hasPanel}
      panelOpen={panelOpen}
      // Bots start bots from their own panel, not threads.
      newChat={hasPanel && !panelOpen && tab !== '/bots'}
      isMobile={isMobile}
      onToggleSidebar={() => setSidebarOpen((open) => !open)}
      onNavigate={handleBrowserNavigation}
    />
  )
  // Settings covers the page, so the page's own title and actions go too.
  const titleRow = settingsOpen ? (
    <span className="ml-3 text-[13px] font-medium text-ink-2">Settings</span>
  ) : (
    <>
      <div id="titlebar-slot" className="relative z-shell ml-3 flex min-w-0 flex-1 items-center gap-1.5">
        <TitlebarSlotOutlet />
      </div>
      <div id="titlebar-actions" className="relative z-shell ml-auto flex min-w-0 items-center gap-1.5">
        <TitlebarActionsOutlet />
      </div>
    </>
  )

  return (
    <TitlebarProvider>
      <ToastProvider>
        <div className="app-chrome flex h-full flex-col">
          {isMobile && (
            <div className="titlebar-drag flex h-[52px] shrink-0 items-center pl-2 pr-3">
              {navigation}
              {titleRow}
            </div>
          )}

          <div className="relative flex min-h-0 flex-1">
            {/* The rail and the thread list sit on the window itself; the
                content floats beside them as cards. */}
            <div className="flex shrink-0 flex-col">
              {isMacDesktop && !isMobile && <div className="titlebar-drag h-10 shrink-0" />}
              <div className="flex min-h-0 flex-1">
                <NavRail tab={tab} onOpenChat={() => setSidebarOpen(true)} onOpenSettings={openSettings} />
                <motion.div
                  className={`shrink-0 overflow-hidden max-sm:absolute max-sm:inset-y-0 max-sm:left-0 max-sm:z-drawer max-sm:w-full! ${hasPanel ? '' : 'hidden'}`}
                  initial={false}
                  animate={drawerSlide({ isMobile, open: sidebarOpen, side: 'left', width: sidebarWidth })}
                  transition={slide}
                >
                  <motion.div
                    className="h-full"
                    initial={false}
                    animate={isMobile || sidebarOpen ? { opacity: 1, x: 0 } : { opacity: 0, x: -12 }}
                    transition={slide}
                  >
                    <Sidebar
                      width={sidebarWidth}
                      mobile={isMobile}
                      onDismiss={() => setSidebarOpen(false)}
                      resizing={resizing}
                      onResizeStart={startResize}
                      onResizeReset={() => setSidebarWidth(SIDEBAR_DEFAULT_WIDTH)}
                      onOpenSettings={() => openSettings()}
                      onOpenConnect={() => setConnectOpen(true)}
                    >
                      {tab === '/bots' ? (
                        <BotsPanel mobile={isMobile} />
                      ) : (
                        <ChatPanel open={panelOpen && tab === 'chat'} mobile={isMobile} onOpenCommandPalette={() => setCommandOpen(true)} />
                      )}
                    </Sidebar>
                  </motion.div>
                </motion.div>
              </div>
            </div>

            <div ref={setPanelRow} className="flex min-w-0 flex-1 sm:pt-2">
              {/* --titlebar-height is the floating title row's height; content
                  starts below it and may scroll beneath it. */}
              <div className="relative flex min-w-0 flex-1 overflow-hidden bg-bg sm:mr-2 sm:mb-2 sm:rounded-[14px] sm:shadow-[var(--shadow-raised)] sm:ring-1 sm:ring-border/80 sm:[--titlebar-height:2.5rem]">
                {!isMobile && (
                  <div
                    // Settings sits below the row, so nothing should show through it there.
                    className={`titlebar-drag absolute inset-x-0 top-0 z-shell flex h-10 items-center pr-3 pl-2 ${settingsOpen ? 'bg-bg' : `bg-gradient-to-b to-transparent from-60% ${onBoard ? 'from-surface' : 'from-bg'}`}`}
                  >
                    {navigation}
                    {titleRow}
                  </div>
                )}
                <main className={`scrollbar-quiet min-w-0 flex-1 overflow-y-auto pt-[var(--titlebar-height,0px)] ${onBoard ? 'bg-surface' : 'bg-bg'}`}>
                  <SidebarVisibility.Provider value={setSidebarOpen}>
                    <SidePanelSlot.Provider value={sidePanelSlot}>
                      <Outlet />
                    </SidePanelSlot.Provider>
                  </SidebarVisibility.Provider>
                </main>

                <MCPApps />

                <SettingsOverlay
                  open={settingsOpen}
                  section={settingsSection}
                  onSectionChange={goToSettingsSection}
                  onClose={closeSettings}
                  onOpenConnect={() => setConnectOpen(true)}
                />
              </div>
              {/* A thread's side panel floats here as its own card. */}
              <div ref={setPanelSlot} className="contents" />
            </div>
          </div>
        </div>
        <ConnectOverlay open={connectOpen} onClose={() => setConnectOpen(false)} />
        <CommandPalette
          open={commandOpen}
          onOpenChange={setCommandOpen}
          onOpenSettings={openSettings}
          onOpenConnect={() => setConnectOpen(true)}
        />
        <GlobalVoice />
      </ToastProvider>
    </TitlebarProvider>
  )
}

function browserNavigationDirection(event: KeyboardEvent): BrowserNavigationDirection | null {
  if (event.key === 'BrowserBack') return 'back'
  if (event.key === 'BrowserForward') return 'forward'
  if (!event.metaKey || event.shiftKey || event.ctrlKey || event.altKey) return null
  if (event.key === '[' || event.code === 'BracketLeft') return 'back'
  if (event.key === ']' || event.code === 'BracketRight') return 'forward'
  return null
}
