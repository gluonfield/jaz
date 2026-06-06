import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Archive } from 'lucide-react'
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

function StatusDot({ status }: { status: Session['status'] }) {
  if (status === 'running') {
    return (
      <span
        title="Running"
        className="size-1.5 shrink-0 animate-pulse rounded-full bg-running"
      />
    )
  }
  if (status === 'error') {
    return <span title="Failed" className="size-1.5 shrink-0 rounded-full bg-danger" />
  }
  return null
}

export function SessionRow({ session, indented = false }: { session: Session; indented?: boolean }) {
  return (
    <Link
      to="/sessions/$sessionId"
      params={{ sessionId: session.id }}
      className={`group flex items-center gap-2 rounded-control px-2.5 py-2 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink ${
        indented ? 'pl-6' : ''
      }`}
      activeProps={{ className: 'bg-primary-soft! text-ink! font-medium' }}
    >
      <StatusDot status={session.status} />
      <span className="min-w-0 flex-1 truncate" title={sessionLabel(session)}>
        {sessionLabel(session)}
      </span>
      {/* native is the default; only agent-backed sessions earn a badge */}
      {session.runtime === 'acp' ? <RuntimeBadge session={session} /> : null}
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
