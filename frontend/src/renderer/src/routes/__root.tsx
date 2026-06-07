import { Outlet, createRootRoute, useRouterState } from '@tanstack/react-router'
import { PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { motion } from 'motion/react'
import { useEffect, useState } from 'react'
import { Sidebar } from '@/components/sidebar/Sidebar'
import { ToastProvider } from '@/components/ui/toast'

export const Route = createRootRoute({
  component: RootLayout,
})

const SIDEBAR_WIDTH = 264
const SIDEBAR_PREF_KEY = 'jaz.sidebar'

function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const [sidebarOpen, setSidebarOpen] = useState(
    () => localStorage.getItem(SIDEBAR_PREF_KEY) !== 'closed',
  )

  useEffect(() => {
    localStorage.setItem(SIDEBAR_PREF_KEY, sidebarOpen ? 'open' : 'closed')
  }, [sidebarOpen])

  // Cmd+S toggles the sidebar — unless something closer (the agent-file
  // editor's save keymap) already claimed the event.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's' && !e.defaultPrevented) {
        e.preventDefault()
        setSidebarOpen((open) => !open)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  return (
    <ToastProvider>
      <div className="flex h-full bg-bg">
        <motion.div
          className="shrink-0 overflow-hidden"
          initial={false}
          animate={{ width: sidebarOpen ? SIDEBAR_WIDTH : 0 }}
          transition={{ type: 'spring', stiffness: 400, damping: 36 }}
        >
          <Sidebar />
        </motion.div>

        <main className="flex min-w-0 flex-1 flex-col">
          <div className="titlebar-drag flex h-[52px] shrink-0 items-center gap-2 px-3">
            <button
              type="button"
              aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
              title={`${sidebarOpen ? 'Hide' : 'Show'} sidebar (⌘S)`}
              onClick={() => setSidebarOpen((open) => !open)}
              className={`grid size-8 cursor-pointer place-items-center rounded-control text-ink-2 transition-all duration-200 hover:bg-surface-2 hover:text-ink ${
                sidebarOpen ? '' : 'ml-[64px]'
              }`}
            >
              {sidebarOpen ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
            </button>
            {/* slot routes portal into (e.g. the session runtime tag) */}
            <div id="titlebar-slot" className="flex items-center" />
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
    </ToastProvider>
  )
}
