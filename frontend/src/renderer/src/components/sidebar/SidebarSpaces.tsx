import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronRight, Folder, Lock, Plus, Settings2, SquarePen, Users } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { ActiveRowHighlight } from '@/components/sidebar/SidebarScroll'
import { SessionRow } from '@/components/sidebar/SessionRow'
import { SpaceModal } from '@/components/sidebar/SpaceModal'
import { Collapse } from '@/components/ui/Collapse'
import { spacesQuery, type CloudSpace } from '@/lib/api/cloudSpaces'
import type { Session } from '@/lib/api/types'
import { useShowModelIcons } from '@/lib/appearance'
import { keys } from '@/lib/query/keys'

const EXPANDED_PROJECTS_KEY = 'jaz.sidebar.expandedCloudProjects'
const INLINE_CHAT_LIMIT = 6

function loadExpandedProjects(): Set<string> {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(EXPANDED_PROJECTS_KEY) ?? '[]')
    return new Set(Array.isArray(raw) ? raw.filter((id): id is string => typeof id === 'string') : [])
  } catch {
    return new Set()
  }
}

export function SidebarSpaces({ sessions }: { sessions: Session[] }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const showRuntimeBadge = useShowModelIcons()
  const spaces = useQuery(spacesQuery)
  const [expanded, setExpanded] = useState<Set<string>>(loadExpandedProjects)
  const [modal, setModal] = useState<{ mode: 'create' } | { mode: 'edit'; space: CloudSpace } | null>(null)

  useEffect(() => {
    try {
      localStorage.setItem(EXPANDED_PROJECTS_KEY, JSON.stringify([...expanded]))
    } catch {
      // Ignore storage quota / privacy mode errors.
    }
  }, [expanded])

  const list = spaces.data ?? []
  const chatsBySpace = useMemo(() => {
    const map = new Map<string, Session[]>()
    for (const session of sessions) {
      if (!session.space_id || session.archived) continue
      const bucket = map.get(session.space_id)
      if (bucket) bucket.push(session)
      else map.set(session.space_id, [session])
    }
    return map
  }, [sessions])

  const toggle = (spaceId: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(spaceId)) next.delete(spaceId)
      else next.add(spaceId)
      return next
    })
  }

  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex h-[30px] items-center justify-between pl-2.5 pr-1 max-sm:h-11 max-sm:pl-3">
        <p className="text-[13px] font-medium text-ink-3 max-sm:text-[15px]">Projects</p>
        <button
          type="button"
          aria-label="New project"
          title="New project"
          onClick={() => setModal({ mode: 'create' })}
          className="grid size-6 cursor-pointer place-items-center rounded-md text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink"
        >
          <Plus size={13} />
        </button>
      </div>

      {list.length === 0 && !spaces.isPending ? (
        <button
          type="button"
          onClick={() => setModal({ mode: 'create' })}
          className="flex h-[30px] cursor-pointer items-center gap-2 rounded-lg px-2.5 text-left text-[12.5px] text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink"
        >
          <Folder size={13} className="shrink-0" />
          <span>New project</span>
        </button>
      ) : null}

      {list.map((space) => {
        const isExpanded = expanded.has(space.id)
        const chats = chatsBySpace.get(space.id) ?? []
        const visibleChats = chats.slice(0, INLINE_CHAT_LIMIT)
        const isTeam = space.visibility === 'team'
        const isPrivate = space.visibility === 'private'
        const VisibilityIcon = isPrivate ? Lock : isTeam ? Users : Folder
        return (
          <div key={space.id} className="flex flex-col">
            <div className="group/space relative flex h-[30px] items-center rounded-lg pr-1 transition-colors duration-150 hover:bg-list-hover max-sm:h-11">
              <button
                type="button"
                aria-expanded={isExpanded}
                aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${space.name}`}
                onClick={() => toggle(space.id)}
                className="ml-1 grid size-5 shrink-0 cursor-pointer place-items-center rounded text-ink-3 transition-colors hover:bg-surface-2 hover:text-ink"
              >
                <ChevronRight
                  size={12}
                  className={`transition-transform duration-150 ${isExpanded ? 'rotate-90' : ''}`}
                />
              </button>
              <Link
                to="/spaces/$spaceId"
                params={{ spaceId: space.id }}
                className="relative flex h-full min-w-0 flex-1 items-center gap-1.5 rounded-lg px-1.5 text-[13px] text-ink-2 transition-colors hover:text-ink max-sm:text-[15px]"
                activeProps={{ className: 'font-medium text-ink' }}
              >
                {({ isActive }) => (
                  <>
                    {isActive ? <ActiveRowHighlight layoutId="sidebar-session" /> : null}
                    <span
                      className="grid size-[16px] shrink-0 place-items-center"
                      title={isPrivate ? 'Private project' : isTeam ? 'Team project' : 'Shared project'}
                    >
                      <VisibilityIcon size={13} className={isActive ? 'text-primary' : 'text-ink-3'} />
                    </span>
                    <span className="min-w-0 flex-1 truncate">{space.name}</span>
                    {chats.length > 0 ? (
                      <span className="shrink-0 text-[11px] text-ink-3 tabular-nums group-hover/space:hidden">
                        {chats.length}
                      </span>
                    ) : null}
                  </>
                )}
              </Link>
              <button
                type="button"
                aria-label={`Project settings for ${space.name}`}
                title={space.can_edit !== false ? 'Project settings' : 'View access'}
                onClick={() => setModal({ mode: 'edit', space })}
                className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/space:opacity-100 [@media(hover:none)]:opacity-100"
              >
                <Settings2 size={12} />
              </button>
              <Link
                to="/new"
                search={{ space: space.id }}
                aria-label={`New chat in ${space.name}`}
                title={`New chat in ${space.name}`}
                className="grid size-6 shrink-0 place-items-center rounded-md text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/space:opacity-100 [@media(hover:none)]:opacity-100"
              >
                <SquarePen size={12} />
              </Link>
            </div>

            <Collapse open={isExpanded}>
              <div className="ml-3.5 border-l border-border/60 pl-1.5 py-0.5">
                {visibleChats.length === 0 ? (
                  <p className="px-2 py-1 text-[11.5px] text-ink-3">No chats yet</p>
                ) : (
                  <div className="flex flex-col gap-px">
                    {visibleChats.map((session) => (
                      <SessionRow
                        key={session.id}
                        session={session}
                        showRuntimeBadge={showRuntimeBadge}
                        reserveIconSpace={false}
                      />
                    ))}
                    {chats.length > INLINE_CHAT_LIMIT ? (
                      <Link
                        to="/spaces/$spaceId"
                        params={{ spaceId: space.id }}
                        className="px-2 py-1 text-[11.5px] text-ink-3 transition-colors hover:text-ink"
                      >
                        View all {chats.length} chats →
                      </Link>
                    ) : null}
                  </div>
                )}
              </div>
            </Collapse>
          </div>
        )
      })}

      {modal ? (
        <SpaceModal
          space={modal.mode === 'edit' ? modal.space : undefined}
          onClose={() => setModal(null)}
          onCreated={(created) => {
            queryClient.setQueryData<CloudSpace[]>(keys.cloudSpaces, (prev) => [
              ...(prev ?? []),
              created,
            ])
            void navigate({ to: '/spaces/$spaceId', params: { spaceId: created.id } })
          }}
          onDeleted={(deletedId) => {
            setExpanded((prev) => {
              if (!prev.has(deletedId)) return prev
              const next = new Set(prev)
              next.delete(deletedId)
              return next
            })
            void navigate({ to: '/new' })
          }}
        />
      ) : null}
    </div>
  )
}
