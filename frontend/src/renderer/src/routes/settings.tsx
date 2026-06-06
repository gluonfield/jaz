import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ArchiveRestore } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { sessionLabel } from '@/components/sidebar/SessionRow'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { archivedSessionsQuery, setSessionArchived } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'

export const Route = createFileRoute('/settings')({
  component: SettingsPage,
})

function SettingsPage() {
  const [showArchived, setShowArchived] = useState(false)

  return (
    <div className="mx-auto max-w-[640px] px-10 pb-12">
      <h1 className="pb-6 text-lg font-semibold text-ink">Settings</h1>

      <section className="flex items-center justify-between gap-4 border-t border-border py-4">
        <div>
          <p className="text-sm font-medium text-ink">Archived chats</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            Threads you archived from the sidebar. Unarchiving puts them back.
          </p>
        </div>
        <button
          type="button"
          onClick={() => setShowArchived((open) => !open)}
          className="shrink-0 cursor-pointer rounded-control border border-border bg-bg px-3 py-1.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface"
        >
          {showArchived ? 'Hide' : 'Show'} archived
        </button>
      </section>

      <AnimatePresence initial={false}>
        {showArchived ? (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
            className="overflow-hidden"
          >
            <ArchivedList />
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

function ArchivedList() {
  const archived = useQuery(archivedSessionsQuery)

  if (archived.isPending) return <SkeletonRows count={3} />
  if (archived.isError) {
    return <p className="py-2 text-[13px] text-danger">{archived.error.message}</p>
  }
  if (archived.data.length === 0) {
    return <p className="py-2 text-[13px] text-ink-3">Nothing archived.</p>
  }

  return (
    <div className="flex flex-col gap-px pb-2">
      <AnimatePresence initial={false}>
        {archived.data.map((item) => (
          <motion.div
            key={item.session.id}
            layout
            exit={{ opacity: 0, x: -8 }}
            transition={{ type: 'spring', stiffness: 420, damping: 34 }}
          >
            <ArchivedRow session={item.session} indented={item.indented} />
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  )
}

function ArchivedRow({ session, indented }: { session: Session; indented: boolean }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const unarchive = useMutation({
    mutationFn: () => setSessionArchived(session.id, false),
    onSuccess: () => toast(`Restored ${sessionLabel(session)}`),
    onError: (error: Error) => toast(`Couldn't restore: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      queryClient.invalidateQueries({ queryKey: keys.archivedSessions })
    },
  })

  return (
    <div
      className={`flex items-center gap-2 rounded-control px-2.5 py-2 text-[13px] text-ink-2 ${
        indented ? 'pl-6' : ''
      }`}
    >
      <span className="min-w-0 flex-1 truncate" title={sessionLabel(session)}>
        {sessionLabel(session)}
      </span>
      {session.runtime === 'acp' ? <RuntimeBadge session={session} /> : null}
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3">
        {relativeTime(session.updated_at)}
      </span>
      <button
        type="button"
        aria-label="Unarchive thread"
        title="Unarchive thread"
        disabled={unarchive.isPending}
        onClick={() => unarchive.mutate()}
        className="grid size-6 shrink-0 cursor-pointer place-items-center rounded text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:opacity-50"
      >
        <ArchiveRestore size={14} />
      </button>
    </div>
  )
}
