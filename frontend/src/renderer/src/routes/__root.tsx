import {
  Outlet,
  createRootRoute,
  useNavigate,
  useRouter,
  useRouterState,
} from '@tanstack/react-router'
import { PanelLeft } from 'lucide-react'
import { motion } from 'motion/react'
import {
  type PointerEvent as ReactPointerEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import { ConnectOverlay } from '@/components/connection/ConnectOverlay'
import { CommandPalette } from '@/components/search/CommandPalette'
import { isSettingsSection, type SettingsSection } from '@/components/settings/sections'
import { SettingsOverlay } from '@/components/settings/SettingsOverlay'
import { Sidebar } from '@/components/sidebar/Sidebar'
import { ToastProvider } from '@/components/ui/toast'
import { clientRuntime } from '@/lib/clientRuntime'
import { drawerSlide } from '@/lib/dom/drawer'
import { modalDialogOpen } from '@/lib/dom/modal'
import { isMobileViewport, useIsMobile } from '@/lib/hooks/useIsMobile'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { TitlebarActionsOutlet, TitlebarProvider, TitlebarSlotOutlet } from '@/lib/titlebar'
import type { BrowserNavigationDirection } from '../../../shared/browserNavigation'

type RootSearch = { settings?: SettingsSection }

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
  return <RootLayout />
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

// The macOS traffic lights only exist in the desktop app, which hides the
// native titlebar and redraws its own; the toggle and content header clear
// them there. The browser client (even on a Mac) keeps the OS title bar, so
// there is nothing at the window's top-left to dodge.
const isMacDesktop = clientRuntime.kind === 'electron' && /Mac/.test(navigator.platform)

const clampSidebarWidth = (w: number) =>
  Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(w)))

function RootLayout() {
  const navigate = useNavigate()
  const router = useRouter()

  // Deep links from board windows ("Open loop in Jaz") land here.
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
    (section: SettingsSection) =>
      void navigate({ to: '.', search: (prev) => ({ ...prev, settings: section }), replace: true }),
    [navigate],
  )
  // Leaving settings is just stepping back. But when it's the entry we loaded
  // into (deep link / web reload) there's nothing behind it, so drop the param
  // directly — otherwise back() is a no-op and the user is stuck inside settings.
  const closeSettings = useCallback(() => {
    if (router.history.canGoBack()) router.history.back()
    else void navigate({ to: '.', search: (prev) => ({ ...prev, settings: undefined }), replace: true })
  }, [router, navigate])

  // A specific board paints itself on bg-surface so its tiles blend; the app
  // titlebar inherits main's background, so match main to surface there too —
  // otherwise the titlebar strip reads as a bg-bg seam above the board.
  const onBoard = useRouterState({
    select: (s) => /^\/boards\/.+/.test(s.location.pathname),
  })

  // Phone: the sidebar is a full-screen drawer (CSS `max-sm:w-full`) that slides
  // over the thread rather than a resizable column, and auto-dismisses on
  // navigation to reveal the thread underneath.
  const isMobile = useIsMobile()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
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

  // Cmd+S toggles the sidebar — unless something closer (the agent-file
  // editor's save keymap) already claimed the event. Cmd+N starts a thread.
  // Cmd+K toggles the command palette. Cmd+[ / Cmd+] follow browser history.
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
    if (!e.shiftKey && e.key.toLowerCase() === 's') {
      e.preventDefault()
      setSidebarOpen((open) => !open)
    }
    if (key === 'n') {
      e.preventDefault()
      navigate({ to: '/new' })
    }
    if (key === 'k') {
      if (!commandOpen && modalDialogOpen()) return
      e.preventDefault()
      setCommandOpen((open) => !open)
    }
  })

  return (
    <TitlebarProvider>
      <ToastProvider>
        <div className="relative flex h-full">
          <motion.div
            className="shrink-0 overflow-hidden max-sm:absolute max-sm:inset-y-0 max-sm:left-0 max-sm:z-drawer max-sm:w-full!"
            initial={false}
            animate={drawerSlide({ isMobile, open: sidebarOpen, side: 'left', width: sidebarWidth })}
            transition={resizing ? { duration: 0 } : { type: 'spring', stiffness: 400, damping: 36 }}
          >
            <Sidebar
              open={sidebarOpen}
              width={sidebarWidth}
              mobile={isMobile}
              onDismiss={() => setSidebarOpen(false)}
              resizing={resizing}
              onResizeStart={startResize}
              onResizeReset={() => setSidebarWidth(SIDEBAR_DEFAULT_WIDTH)}
              onOpenSettings={() => openSettings()}
              onOpenConnect={() => setConnectOpen(true)}
            />
          </motion.div>

          <main className={`flex min-w-0 flex-1 flex-col ${onBoard ? 'bg-surface' : 'bg-bg'}`}>
            {/* When collapsed, the content owns the window's top-left, so its
                header indents past the traffic lights and the pinned toggle. */}
            <div
              className={`titlebar-drag flex h-[52px] shrink-0 items-center gap-2 pr-3 ${
                sidebarOpen ? 'pl-3' : isMobile ? 'pl-14' : isMacDesktop ? 'pl-[108px]' : 'pl-12'
              }`}
            >
              <div id="titlebar-slot" className="relative z-shell flex min-w-0 items-center gap-1.5">
                <TitlebarSlotOutlet />
              </div>
              <div id="titlebar-actions" className="relative z-shell ml-auto flex items-center gap-1.5">
                <TitlebarActionsOutlet />
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto">
              <Outlet />
            </div>
          </main>

          {/* Sidebar toggle, pinned beside the macOS traffic lights and sized to
              match them. Kept LAST and explicitly no-drag because Electron unions
              the .titlebar-drag strips then subtracts no-drag rects in document
              order — so this cutout only stays clickable when subtracted after
              the strips it overlaps. */}
          <button
            type="button"
            aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
            title={`${sidebarOpen ? 'Hide' : 'Show'} sidebar (⌘S)`}
            onClick={() => setSidebarOpen((open) => !open)}
            className={`absolute z-drawer grid cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-150 [-webkit-app-region:no-drag] hover:bg-surface-2 hover:text-ink ${
              isMobile
                ? 'top-2.5 left-3 size-9'
                : `top-[11px] size-7 ${isMacDesktop ? 'left-[80px]' : 'left-2'}`
            }`}
          >
            <PanelLeft size={isMobile ? 20 : 16} />
          </button>
        </div>
        <SettingsOverlay
          open={settingsOpen}
          section={settingsSection}
          onSectionChange={goToSettingsSection}
          onClose={closeSettings}
          onOpenConnect={() => setConnectOpen(true)}
        />
        <ConnectOverlay open={connectOpen} onClose={() => setConnectOpen(false)} />
        <CommandPalette
          open={commandOpen}
          onOpenChange={setCommandOpen}
          onOpenSettings={openSettings}
          onOpenConnect={() => setConnectOpen(true)}
        />
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
