import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { SquarePen } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { SIDEBAR_SESSION_LIMIT, sidebarSessionsQuery } from '@/lib/api/sessions'
import { SkeletonRows } from '../ui/Skeleton'
import { SessionRow } from './SessionRow'

function SectionLabel({ children }: { children: string }) {
  return (
    <p className="px-2.5 pb-1.5 text-[11px] font-semibold tracking-wide text-ink-3">{children}</p>
  )
}

function PageLink({ to, label, hint }: { to: string; label: string; hint?: string }) {
  return (
    <Link
      to={to}
      className="flex items-baseline gap-2 rounded-control px-2.5 py-2 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      activeProps={{ className: 'bg-primary-soft! text-ink! font-medium' }}
    >
      <span className="flex-1">{label}</span>
      {hint ? <span className="text-[11px] text-ink-3">{hint}</span> : null}
    </Link>
  )
}

function SoonItem({ label }: { label: string }) {
  return (
    <div
      aria-disabled
      className="flex items-baseline gap-2 rounded-control px-2.5 py-2 text-[13px] text-ink-3"
    >
      <span className="flex-1">{label}</span>
      <span className="rounded bg-surface-2 px-1.5 py-px text-[10px] font-medium">soon</span>
    </div>
  )
}

export function Sidebar() {
  const sessions = useQuery(sidebarSessionsQuery)
  const visibleSessions = sessions.data?.slice(0, SIDEBAR_SESSION_LIMIT) ?? []

  return (
    <aside className="flex h-full w-[264px] shrink-0 flex-col border-r border-border bg-surface">
      {/* draggable titlebar strip; traffic lights live here on macOS */}
      <div className="titlebar-drag h-[52px] shrink-0" />

      <nav className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto p-3 pt-4">
        <Link
          to="/new"
          className="flex items-center gap-2 rounded-control px-2.5 py-2 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2"
          activeProps={{ className: 'bg-primary-soft!' }}
        >
          <SquarePen size={15} className="text-ink-2" />
          New session
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
                {visibleSessions.map((session) => (
                  <motion.div
                    key={session.id}
                    layout
                    initial={{ opacity: 0, x: -8 }}
                    animate={{ opacity: 1, x: 0 }}
                    exit={{ opacity: 0, x: -8 }}
                    transition={{ type: 'spring', stiffness: 420, damping: 34 }}
                  >
                    <SessionRow session={session} />
                  </motion.div>
                ))}
              </AnimatePresence>
              {sessions.data.length > SIDEBAR_SESSION_LIMIT ? (
                <Link
                  to="/sessions"
                  className="mt-1 rounded-control px-2.5 py-1.5 text-[13px] text-primary transition-colors duration-150 hover:bg-surface-2"
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
            <SoonItem label="Settings" />
          </div>
        </section>
      </nav>
    </aside>
  )
}
