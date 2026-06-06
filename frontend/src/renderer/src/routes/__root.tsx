import { Outlet, createRootRoute, useRouterState } from '@tanstack/react-router'
import { motion } from 'motion/react'
import { Sidebar } from '@/components/sidebar/Sidebar'
import { ToastProvider } from '@/components/ui/toast'

export const Route = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <ToastProvider>
      <div className="flex h-full bg-bg">
        <Sidebar />
        <main className="flex min-w-0 flex-1 flex-col">
          {/* draggable strip mirrors the sidebar's titlebar height */}
          <div className="titlebar-drag h-[52px] shrink-0" />
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
