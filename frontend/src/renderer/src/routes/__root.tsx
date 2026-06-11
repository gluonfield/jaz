import {
  Outlet,
  createRootRoute,
  useNavigate,
  useRouter,
} from '@tanstack/react-router'
import { PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { motion } from 'motion/react'
import { type PointerEvent as ReactPointerEvent, useEffect, useState } from 'react'
import { SettingsOverlay } from '@/components/settings/SettingsOverlay'
import { Sidebar } from '@/components/sidebar/Sidebar'
import { ToastProvider } from '@/components/ui/toast'
import { UpdatePanel } from '@/components/update/UpdatePanel'

export const Route = createRootRoute({
  component: RootComponent,
})

// Board windows render their route full-bleed: no sidebar, no app titlebar
// (they use the native OS titlebar). windowKind is fixed per window, so the
// branch never changes within a window's lifetime.
function RootComponent() {
  if (window.jaz?.windowKind === 'board') {
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
  return <RootLayout />
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

  // Deep links from board windows ("Open loop in Jaz") land here.
  useEffect(() => {
    return window.jaz?.onOpenRoute?.((path) => router.history.push(path))
  }, [router])
  const [sidebarOpen, setSidebarOpen] = useState(
    () => localStorage.getItem(SIDEBAR_PREF_KEY) !== 'closed',
  )
  const [sidebarWidth, setSidebarWidth] = useState(() => {
    const stored = Number(localStorage.getItem(SIDEBAR_WIDTH_KEY))
    return stored > 0 ? clampSidebarWidth(stored) : SIDEBAR_DEFAULT_WIDTH
  })
  const [resizing, setResizing] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)

  useEffect(() => {
    localStorage.setItem(SIDEBAR_PREF_KEY, sidebarOpen ? 'open' : 'closed')
  }, [sidebarOpen])

  useEffect(() => {
    localStorage.setItem(SIDEBAR_WIDTH_KEY, String(sidebarWidth))
  }, [sidebarWidth])

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

  // Cmd+S toggles the sidebar — unless something closer (the agent-file
  // editor's save keymap) already claimed the event. Cmd+N starts a thread.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.defaultPrevented) return
      if (!e.shiftKey && e.key.toLowerCase() === 's') {
        e.preventDefault()
        setSidebarOpen((open) => !open)
      }
      if (e.key === 'n') {
        e.preventDefault()
        navigate({ to: '/new' })
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [navigate])

  return (
    <ToastProvider>
      <div className="flex h-full">
        <motion.div
          className="shrink-0 overflow-hidden"
          initial={false}
          animate={{ width: sidebarOpen ? sidebarWidth : 0 }}
          transition={resizing ? { duration: 0 } : { type: 'spring', stiffness: 400, damping: 36 }}
        >
          <Sidebar
            width={sidebarWidth}
            resizing={resizing}
            onResizeStart={startResize}
            onResizeReset={() => setSidebarWidth(SIDEBAR_DEFAULT_WIDTH)}
            onOpenSettings={() => setSettingsOpen(true)}
          />
        </motion.div>

        <main className="flex min-w-0 flex-1 flex-col bg-bg">
          <div className="titlebar-drag flex h-[52px] shrink-0 items-center gap-2 px-3">
            <button
              type="button"
              aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
              title={`${sidebarOpen ? 'Hide' : 'Show'} sidebar (⌘S)`}
              onClick={() => setSidebarOpen((open) => !open)}
              className={`grid size-8 cursor-pointer place-items-center rounded-full text-ink-2 transition-all duration-200 hover:bg-surface-2 hover:text-ink ${
                sidebarOpen ? '' : 'ml-[64px]'
              }`}
            >
              {sidebarOpen ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
            </button>
            {/* slot routes portal into (e.g. the session runtime tag) */}
            <div id="titlebar-slot" className="flex min-w-0 items-center gap-1.5" />
            {/* right-aligned slot for route-level actions */}
            <div id="titlebar-actions" className="ml-auto flex items-center gap-1.5" />
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            <Outlet />
          </div>
        </main>
      </div>
      <SettingsOverlay open={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <UpdatePanel />
    </ToastProvider>
  )
}
