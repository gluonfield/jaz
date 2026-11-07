import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Archive, CornerDownRight, Pencil, Pin } from 'lucide-react'
import { useRef, useState } from 'react'
import { SessionHoverDetails } from '@/components/sidebar/SessionHoverDetails'
import { ActiveRowHighlight } from '@/components/sidebar/SidebarScroll'
import { Button } from '@/components/ui/Button'
import { HoverCard } from '@/components/ui/HoverCard'
import { Input } from '@/components/ui/Input'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { Modal } from '@/components/ui/Modal'
import { ContextMenu, MenuRow } from '@/components/ui/Popover'
import { setSessionArchived, setSessionPinned, setSessionTitle } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import { invalidateSessionLists } from '@/lib/query/invalidate'
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

function isCoarsePointer() {
  return window.matchMedia?.('(pointer: coarse)').matches === true
}

// Running, failed and unread are the same kind of fact, so they share the
// trailing dot column rather than sitting on opposite sides of the row. Unread
// follows the Feed: a finished turn nobody has opened yet.
export function stateDot(session: Pick<Session, 'status' | 'unread' | 'error'>): { title: string; className: string } | null {
  if (session.status === 'running') return { title: 'Running', className: 'animate-pulse bg-running' }
  if (session.status === 'error') {
    return { title: session.error ? `Failed: ${session.error}` : 'Failed', className: 'bg-danger' }
  }
  if (session.status === 'idle' && session.unread) return { title: 'Unread reply', className: 'bg-primary' }
  return null
}

export function SessionRow({
  session,
  child = false,
  shortcutIndex,
  shortcutMode = false,
  showRuntimeBadge = true,
  reserveIconSpace,
}: {
  session: Session
  child?: boolean
  shortcutIndex?: number
  shortcutMode?: boolean
  showRuntimeBadge?: boolean
  reserveIconSpace: boolean
}) {
  const shortcut = shortcutMode && shortcutIndex ? shortcutIndex : undefined
  const title = sessionLabel(session)
  const state = stateDot(session)
  const [rename, setRename] = useState<null | 'inline' | 'modal'>(null)
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const menuTriggers = useContextMenuTrigger(setMenu)
  const startRename = () => setRename(isCoarsePointer() ? 'modal' : 'inline')
  const inlineEditing = rename === 'inline'
  const icon = child ? (
    <CornerDownRight size={12} className="text-ink-3" />
  ) : showRuntimeBadge && session.runtime === 'acp' ? (
    <RuntimeBadge session={session} compact />
  ) : null

  return (
    <>
      <HoverCard
        disabled={rename !== null || menu !== null}
        content={<SessionHoverDetails session={session} title={title} />}
      >
        <Link
          to="/sessions/$sessionId"
          params={{ sessionId: session.id }}
          className="group relative flex h-[30px] select-none items-center gap-2 rounded-lg px-2.5 text-[13px] text-ink transition-colors duration-150 [-webkit-touch-callout:none] max-sm:h-11 max-sm:gap-2.5 max-sm:px-3 max-sm:text-[15px]"
          inactiveProps={{ className: 'hover:bg-list-hover' }}
          {...menuTriggers}
        >
          {({ isActive }) => (
            <>
              {isActive ? <ActiveRowHighlight layoutId="sidebar-session" /> : null}
              {!session.pinned && (reserveIconSpace || icon) && (
                <span className="grid size-[18px] shrink-0 place-items-center">
                  {icon}
                </span>
              )}
              {inlineEditing ? (
                <RenameField session={session} onDone={() => setRename(null)} />
              ) : (
                <span className="min-w-0 flex-1 truncate-fade">
                  {title}
                </span>
              )}
              {inlineEditing ? null : shortcut ? (
                <span className="flex min-w-8 shrink-0 justify-end">
                  <KeyboardShortcut value={shortcut} />
                </span>
              ) : state ? (
                <span
                  className={`flex min-w-8 shrink-0 items-center justify-end ${
                    shortcutMode ? '' : 'group-hover:hidden'
                  }`}
                >
                  <span title={state.title} className={`size-1.5 shrink-0 rounded-full ${state.className}`} />
                </span>
              ) : null}
              {shortcutMode || inlineEditing ? null : (
                <>
                  <PinButton session={session} />
                  <ArchiveButton session={session} />
                </>
              )}
            </>
          )}
        </Link>
      </HoverCard>
      {menu ? (
        <RowActionsMenu
          session={session}
          point={menu}
          onClose={() => setMenu(null)}
          onRename={startRename}
        />
      ) : null}
      {rename === 'modal' ? (
        <RenameDialog session={session} onClose={() => setRename(null)} />
      ) : null}
    </>
  )
}

function useRenameSession(session: Session) {
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: (title: string) => setSessionTitle(session.id, title),
    onSettled: () => invalidateSessionLists(queryClient, { session: session.id }),
  })
  return (title: string) => {
    const next = title.trim()
    if (next && next !== (session.title ?? '')) mutation.mutate(next)
  }
}

// Inline title editor that swaps in for the row label. Enter or blur commits,
// Escape reverts; a guard keeps Enter+blur from firing the mutation twice.
function RenameField({ session, onDone }: { session: Session; onDone: () => void }) {
  const [value, setValue] = useState(session.title ?? '')
  const settled = useRef(false)
  const rename = useRenameSession(session)

  const finish = (commit: boolean) => {
    if (settled.current) return
    settled.current = true
    if (commit) rename(value)
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
      className="min-w-0 flex-1 select-text rounded bg-surface-1 px-1.5 py-0.5 text-[13px] text-ink outline-none ring-1 ring-primary"
    />
  )
}

// Touch-friendly rename: a dialog with an explicit Save/Cancel instead of the
// inline field's commit-on-blur, which is awkward with an on-screen keyboard.
function RenameDialog({ session, onClose }: { session: Session; onClose: () => void }) {
  const [value, setValue] = useState(session.title ?? '')
  const rename = useRenameSession(session)

  const save = () => {
    rename(value)
    onClose()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title="Rename chat"
      size="sm"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={save}>
            Save
          </Button>
        </div>
      }
    >
      <Input
        autoFocus
        value={value}
        placeholder="Name this chat"
        onChange={(e) => setValue(e.target.value)}
        onFocus={(e) => e.currentTarget.select()}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            save()
          }
        }}
      />
    </Modal>
  )
}

function usePinToggle(session: Session) {
  const queryClient = useQueryClient()
  const pinned = Boolean(session.pinned)
  const mutation = useMutation({
    mutationFn: () => setSessionPinned(session.id, !pinned),
    onSettled: () => invalidateSessionLists(queryClient),
  })
  return { pinned, toggle: () => mutation.mutate() }
}

function useArchive(session: Session) {
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: () => setSessionArchived(session.id, true),
    onSettled: () => invalidateSessionLists(queryClient, { archived: true }),
  })
  return () => mutation.mutate()
}

function PinButton({ session }: { session: Session }) {
  const { pinned, toggle } = usePinToggle(session)
  return (
    <button
      type="button"
      aria-label={pinned ? 'Unpin thread' : 'Pin thread'}
      aria-pressed={pinned}
      title={pinned ? 'Unpin thread' : 'Pin thread'}
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        toggle()
      }}
      className={`${pinned ? 'text-primary' : 'text-ink-3'} hidden size-5 shrink-0 cursor-pointer place-items-center rounded transition-colors duration-150 hover:bg-surface-2 hover:text-ink group-hover:grid max-sm:grid max-sm:size-8`}
    >
      <Pin size={13} className={pinned ? 'fill-current' : ''} />
    </button>
  )
}

function ArchiveButton({ session }: { session: Session }) {
  const archive = useArchive(session)
  return (
    <button
      type="button"
      aria-label="Archive thread"
      title="Archive thread"
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        archive()
      }}
      className="hidden size-5 shrink-0 cursor-pointer place-items-center rounded text-ink-3 transition-colors duration-150 group-hover:grid hover:bg-surface-2 hover:text-ink max-sm:grid max-sm:size-8"
    >
      <Archive size={13} />
    </button>
  )
}

// Right-click / press-and-hold actions for a sidebar row. Pin/archive cascade to
// child threads; rename hands back to the row's inline editor or touch dialog.
function RowActionsMenu({
  session,
  point,
  onClose,
  onRename,
}: {
  session: Session
  point: { x: number; y: number }
  onClose: () => void
  onRename: () => void
}) {
  const { pinned, toggle: togglePin } = usePinToggle(session)
  const archive = useArchive(session)

  const run = (fn: () => void) => {
    onClose()
    fn()
  }

  return (
    <ContextMenu point={point} onClose={onClose}>
      <MenuRow onClick={() => run(onRename)}>
        <span className="flex items-center gap-2">
          <Pencil size={13} />
          Rename
        </span>
      </MenuRow>
      <MenuRow onClick={() => run(togglePin)}>
        <span className="flex items-center gap-2">
          <Pin size={13} className={pinned ? 'fill-current' : ''} />
          {pinned ? 'Unpin' : 'Pin'}
        </span>
      </MenuRow>
      <MenuRow onClick={() => run(archive)}>
        <span className="flex items-center gap-2">
          <Archive size={13} />
          Archive
        </span>
      </MenuRow>
    </ContextMenu>
  )
}
