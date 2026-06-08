import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { SquarePen } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { SIDEBAR_SESSION_LIMIT, sidebarSessionsQuery } from '@/lib/api/sessions'
import { SkeletonRows } from '../ui/Skeleton'
import { SessionRow } from './SessionRow'
import { ThemeSwitcher } from './ThemeSwitcher'

function SectionLabel({ children }: { children: string }) {
  return (
    <p className="px-2 pb-1 text-[11px] font-semibold tracking-wide text-ink-3">{children}</p>
  )
}

function PageLink({ to, label, hint }: { to: string; label: string; hint?: string }) {
  return (
    <Link
      to={to}
      className="flex items-baseline gap-2 rounded-control px-2 py-1.5 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      activeProps={{ className: 'bg-primary-soft! text-ink! font-medium' }}
    >
      <span className="flex-1">{label}</span>
      {hint ? <span className="text-[11px] text-ink-3">{hint}</span> : null}
    </Link>
  )
}

export function Sidebar() {
  const sessions = useQuery(sidebarSessionsQuery)
  const visibleSessions = sessions.data?.slice(0, SIDEBAR_SESSION_LIMIT) ?? []

  return (
    <aside className="flex h-full w-[264px] shrink-0 flex-col border-r border-border bg-surface">
      {/* draggable titlebar strip; traffic lights live here on macOS */}
      <div className="titlebar-drag h-[52px] shrink-0" />

      <nav className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-3 pt-3">
        <Link
          to="/new"
          className="group flex items-center gap-2 rounded-control px-2 py-1.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2"
          activeProps={{ className: 'bg-primary-soft!' }}
        >
          <SquarePen size={15} className="text-ink-2" />
          <span className="flex-1">New Thread</span>
          <span className="flex items-center gap-0.5 text-[10px] text-ink-3">
            <kbd className="rounded border border-border bg-bg px-1 font-sans">⌘</kbd>
            <kbd className="rounded border border-border bg-bg px-1 font-sans">N</kbd>
          </span>
        </Link>

        <section>
          <SectionLabel>Sessions</SectionLabel>
          {sessions.isPending ? (
            <SkeletonRows count={4} />
          ) : sessions.isError ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
          ) : visibleSessions.length === 0 ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">No sessions yet</p>
          ) : (
            <div className="flex flex-col gap-px">
              <AnimatePresence initial={false}>
                {visibleSessions.map((item) => (
                  <motion.div
                    key={item.session.id}
                    layout
                    initial={{ opacity: 0, x: -8 }}
                    animate={{ opacity: 1, x: 0 }}
                    exit={{ opacity: 0, x: -8 }}
                    transition={{ type: 'spring', stiffness: 420, damping: 34 }}
                  >
                    <SessionRow session={item.session} child={item.child} />
                  </motion.div>
                ))}
              </AnimatePresence>
              {sessions.data.length > SIDEBAR_SESSION_LIMIT ? (
                <Link
                  to="/sessions"
                  className="mt-1 rounded-control px-2 py-1 text-[13px] text-primary transition-colors duration-150 hover:bg-surface-2"
                  activeOptions={{ exact: true }}
                  activeProps={{ className: 'bg-primary-soft!' }}
                >
                  Show all sessions
                </Link>
              ) : null}
            </div>
          )}
        </section>

        <section>
          <SectionLabel>Pages</SectionLabel>
          <div className="flex flex-col gap-px">
            <PageLink to="/agent" label="Agent" />
            <PageLink to="/settings" label="Settings" />
          </div>
        </section>
      </nav>

      <div className="shrink-0 border-t border-border p-3">
        <ThemeSwitcher />
      </div>
    </aside>
  )
}
