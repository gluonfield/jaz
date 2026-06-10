import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArchiveRestore, CornerDownRight } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { sessionLabel } from '@/components/sidebar/SessionRow'
import { IconButton } from '@/components/ui/IconButton'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { archivedSessionsQuery, setSessionArchived } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'

export function ArchivedThreadsSettings() {
  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">Archived threads</p>
          <p className="mt-0.5 text-[13px] text-ink-2">Threads archived from the sidebar.</p>
        </div>
      </div>

      <div className="mt-4">
        <AnimatePresence initial={false}>
          <motion.div
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
          >
            <ArchivedList />
          </motion.div>
        </AnimatePresence>
      </div>
    </section>
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
      <AnimatePresence initial={false} mode="popLayout">
        {archived.data.map((item) => (
          <motion.div
            key={item.session.id}
            exit={{ opacity: 0, x: -8 }}
            transition={{ type: 'spring', stiffness: 420, damping: 34 }}
          >
            <ArchivedRow session={item.session} child={item.child} />
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  )
}

function ArchivedRow({ session, child = false }: { session: Session; child?: boolean }) {
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
    <div className="flex items-center gap-2 rounded-full px-3 py-2 text-[13px] text-ink-2">
      {child ? <CornerDownRight size={12} className="shrink-0 text-ink-3" /> : null}
      {session.runtime === 'acp' ? (
        <RuntimeBadge session={session} className={child ? '' : '-ml-1.5'} />
      ) : null}
      <span className="min-w-0 flex-1 truncate" title={sessionLabel(session)}>
        {sessionLabel(session)}
      </span>
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3">
        {relativeTime(session.updated_at)}
      </span>
      <IconButton
        variant="ghost"
        size="xs"
        aria-label="Unarchive thread"
        title="Unarchive thread"
        disabled={unarchive.isPending}
        onClick={() => unarchive.mutate()}
      >
        <ArchiveRestore size={14} />
      </IconButton>
    </div>
  )
}
