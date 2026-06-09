import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Archive, CornerDownRight } from 'lucide-react'
import { setSessionArchived } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'
import { RuntimeBadge } from './RuntimeBadge'

// Auto-generated chat slugs (chat-2026-06-06-153125) carry no scannable
// signal; turn them into a friendly date label instead.
export function sessionLabel(session: Session): string {
  if (session.title) return session.title
  if (session.slug && !session.slug.startsWith('chat-')) return session.slug
  const created = new Date(session.created_at)
  if (Number.isNaN(created.getTime())) return session.slug || session.id
  const sameDay = created.toDateString() === new Date().toDateString()
  const when = sameDay
    ? created.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
    : created.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  return `Chat · ${when}`
}

function StatusDot({ session }: { session: Session }) {
  if (session.status === 'running') {
    return (
      <span
        title="Running"
        className="size-1.5 shrink-0 animate-pulse rounded-full bg-running"
      />
    )
  }
  if (session.status === 'error') {
    return (
      <span
        title={session.error ? `Failed: ${session.error}` : 'Failed'}
        className="size-1.5 shrink-0 rounded-full bg-danger"
      />
    )
  }
  return null
}

export function SessionRow({ session, child = false }: { session: Session; child?: boolean }) {
  return (
    <Link
      to="/sessions/$sessionId"
      params={{ sessionId: session.id }}
      className="group flex h-8 items-center gap-2 rounded-control px-2 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      activeProps={{ className: 'bg-primary-soft! text-ink! font-medium' }}
    >
      {/* branch connector: this thread was spawned by the session above */}
      {child ? <CornerDownRight size={12} className="shrink-0 text-ink-3" /> : null}
      <StatusDot session={session} />
      {/* native is the default; only agent-backed sessions earn a badge.
          When the chip leads the row, a negative margin optically aligns
          its text with the titles. */}
      {session.runtime === 'acp' ? (
        <RuntimeBadge session={session} compact className={child ? '' : '-ml-1.5'} />
      ) : null}
      <span className="min-w-0 flex-1 truncate" title={sessionLabel(session)}>
        {sessionLabel(session)}
      </span>
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3 group-hover:hidden">
        {relativeTime(session.updated_at)}
      </span>
      <ArchiveButton sessionId={session.id} />
    </Link>
  )
}

// Replaces the timestamp on row hover; archives the thread (and children).
function ArchiveButton({ sessionId }: { sessionId: string }) {
  const queryClient = useQueryClient()
  const archive = useMutation({
    mutationFn: () => setSessionArchived(sessionId, true),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      queryClient.invalidateQueries({ queryKey: keys.archivedSessions })
    },
  })

  return (
    <button
      type="button"
      aria-label="Archive thread"
      title="Archive thread"
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        archive.mutate()
      }}
      className="hidden size-5 shrink-0 cursor-pointer place-items-center rounded text-ink-3 transition-colors duration-150 group-hover:grid hover:bg-surface-2 hover:text-ink"
    >
      <Archive size={13} />
    </button>
  )
}
