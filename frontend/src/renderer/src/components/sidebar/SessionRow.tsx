import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Archive, CornerDownRight, Pencil, Pin } from 'lucide-react'
import { useRef, useState } from 'react'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { setSessionArchived, setSessionPinned, setSessionTitle } from '@/lib/api/sessions'
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

export function SessionRow({
  session,
  child = false,
  shortcutIndex,
  shortcutMode = false,
}: {
  session: Session
  child?: boolean
  shortcutIndex?: number
  shortcutMode?: boolean
}) {
  const shortcut = shortcutMode && shortcutIndex ? shortcutIndex : undefined
  const [editing, setEditing] = useState(false)

  return (
    <Link
      to="/sessions/$sessionId"
      params={{ sessionId: session.id }}
      className="group flex h-8 items-center gap-2 rounded-full px-2.5 text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
      activeProps={{ className: 'bg-primary-soft! text-ink! font-medium' }}
    >
      {/* branch connector: this thread was spawned by the session above */}
      {child ? <CornerDownRight size={12} className="shrink-0 text-ink-3" /> : null}
      <StatusDot session={session} />
      {/* When the chip leads the row, a negative margin optically aligns
          its text with the titles. */}
      {session.runtime === 'acp' ? (
        <RuntimeBadge session={session} compact className={child ? '' : '-ml-1.5'} />
      ) : null}
      {editing ? (
        <RenameField session={session} onDone={() => setEditing(false)} />
      ) : (
        <span
          className="min-w-0 flex-1 truncate"
          title={sessionLabel(session)}
          onDoubleClick={(e) => {
            e.preventDefault()
            e.stopPropagation()
            setEditing(true)
          }}
        >
          {sessionLabel(session)}
        </span>
      )}
      {editing ? null : shortcut ? (
        <span className="flex min-w-8 shrink-0 justify-end">
          <KeyboardShortcut value={shortcut} />
        </span>
      ) : session.status === 'running' ? (
        <span
          title="Running"
          className={`flex min-w-8 shrink-0 items-center justify-end ${
            shortcutMode ? '' : 'group-hover:hidden'
          }`}
        >
          <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-running" />
        </span>
      ) : (
        <span
          className={`min-w-8 shrink-0 text-right text-[11px] tabular-nums ${
            shortcutMode ? 'text-ink-3' : 'text-ink-3 group-hover:hidden'
          }`}
        >
          {relativeTime(session.last_attention_at || session.updated_at)}
        </span>
      )}
      {shortcutMode || editing ? null : (
        <>
          <RenameButton onStart={() => setEditing(true)} />
          <PinButton session={session} />
          <ArchiveButton sessionId={session.id} />
        </>
      )}
    </Link>
  )
}

// Inline title editor that swaps in for the row label. Enter or blur commits,
// Escape reverts; a guard keeps Enter+blur from firing the mutation twice.
function RenameField({ session, onDone }: { session: Session; onDone: () => void }) {
  const queryClient = useQueryClient()
  const [value, setValue] = useState(session.title ?? '')
  const settled = useRef(false)
  const rename = useMutation({
    mutationFn: (title: string) => setSessionTitle(session.id, title),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      queryClient.invalidateQueries({ queryKey: keys.session(session.id), exact: true })
    },
  })

  const finish = (commit: boolean) => {
    if (settled.current) return
    settled.current = true
    const next = value.trim()
    if (commit && next && next !== (session.title ?? '')) rename.mutate(next)
    onDone()
  }

  return (
    <input
      autoFocus
      value={value}
      placeholder="Name this chat"
      onChange={(e) => setValue(e.target.value)}
      onFocus={(e) => e.currentTarget.select()}
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
      }}
      onKeyDown={(e) => {
        e.stopPropagation()
        if (e.key === 'Enter') {
          e.preventDefault()
          finish(true)
        } else if (e.key === 'Escape') {
          e.preventDefault()
          finish(false)
        }
      }}
      onBlur={() => finish(true)}
      className="min-w-0 flex-1 rounded bg-surface-1 px-1.5 py-0.5 text-[13px] text-ink outline-none ring-1 ring-primary"
    />
  )
}

function RenameButton({ onStart }: { onStart: () => void }) {
  return (
    <button
      type="button"
      aria-label="Rename thread"
      title="Rename thread"
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        onStart()
      }}
      className="hidden size-5 shrink-0 cursor-pointer place-items-center rounded text-ink-3 transition-colors duration-150 group-hover:grid hover:bg-surface-2 hover:text-ink"
    >
      <Pencil size={13} />
    </button>
  )
}

function PinButton({ session }: { session: Session }) {
  const queryClient = useQueryClient()
  const pinned = Boolean(session.pinned)
  const pin = useMutation({
    mutationFn: () => setSessionPinned(session.id, !pinned),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
    },
  })

  return (
    <button
      type="button"
      aria-label={pinned ? 'Unpin thread' : 'Pin thread'}
      aria-pressed={pinned}
      title={pinned ? 'Unpin thread' : 'Pin thread'}
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        pin.mutate()
      }}
      className={`${pinned ? 'text-primary' : 'text-ink-3'} hidden size-5 shrink-0 cursor-pointer place-items-center rounded transition-colors duration-150 hover:bg-surface-2 hover:text-ink group-hover:grid`}
    >
      <Pin size={13} className={pinned ? 'fill-current' : ''} />
    </button>
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
