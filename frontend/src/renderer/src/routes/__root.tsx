import { Outlet, createRootRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { motion } from 'motion/react'
import { type PointerEvent as ReactPointerEvent, useEffect, useState } from 'react'
import { SettingsOverlay } from '@/components/settings/SettingsOverlay'
import { Sidebar } from '@/components/sidebar/Sidebar'
import { ToastProvider } from '@/components/ui/toast'

export const Route = createRootRoute({
  component: RootLayout,
})

const SIDEBAR_DEFAULT_WIDTH = 264
const SIDEBAR_MIN_WIDTH = 200
const SIDEBAR_MAX_WIDTH = 480
const SIDEBAR_PREF_KEY = 'jaz.sidebar'
const SIDEBAR_WIDTH_KEY = 'jaz.sidebarWidth'

const clampSidebarWidth = (w: number) =>
  Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(w)))

function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const navigate = useNavigate()
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
      if (e.key === 's') {
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
      <div className="flex h-full bg-bg">
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

        <main className="flex min-w-0 flex-1 flex-col">
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
            <div id="titlebar-slot" className="flex items-center gap-1.5" />
          </div>
          {/* light crossfade between routes; state-only, never blocking */}
          <motion.div
            key={pathname}
            className="min-h-0 flex-1 overflow-y-auto"
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.15, ease: 'easeOut' }}
          >
            <Outlet />
          </motion.div>
        </main>
      </div>
      <SettingsOverlay open={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </ToastProvider>
  )
}
