import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Lock, Plus, SquarePen, UserPlus, Users } from 'lucide-react'
import { useState } from 'react'
import { SpaceModal } from '@/components/sidebar/SpaceModal'
import { spacesQuery, type CloudSpace } from '@/lib/api/cloudSpaces'
import { keys } from '@/lib/query/keys'

export function SidebarSpaces({
  selectedSpaceId,
  onSelectSpace,
}: {
  selectedSpaceId: string | null
  onSelectSpace: (spaceId: string | null) => void
}) {
  const queryClient = useQueryClient()
  const spaces = useQuery(spacesQuery)
  const [modal, setModal] = useState<{ mode: 'create' } | { mode: 'edit'; space: CloudSpace } | null>(
    null,
  )

  const list = spaces.data ?? []

  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex h-[30px] items-center justify-between pl-2.5 pr-1 max-sm:h-11 max-sm:pl-3">
        <p className="text-[13px] font-medium text-ink-3 max-sm:text-[15px]">Spaces</p>
        <div className="flex items-center gap-0.5">
          {selectedSpaceId ? (
            <button
              type="button"
              onClick={() => onSelectSpace(null)}
              className="cursor-pointer rounded-md px-1.5 py-0.5 text-[11px] text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink"
            >
              All
            </button>
          ) : null}
          <button
            type="button"
            aria-label="New space"
            title="New space"
            onClick={() => setModal({ mode: 'create' })}
            className="grid size-6 cursor-pointer place-items-center rounded-md text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink"
          >
            <Plus size={13} />
          </button>
        </div>
      </div>

      {list.length === 0 && !spaces.isPending ? (
        <p className="px-2.5 py-1 text-[12px] text-ink-3">No shared spaces yet</p>
      ) : null}

      {list.map((space) => {
        const active = selectedSpaceId === space.id
        const isSelectedOnly = space.visibility === 'selected'
        const SpaceIcon = isSelectedOnly ? Lock : Users
        const count = isSelectedOnly ? (space.member_count ?? space.members?.length) : undefined
        return (
          <div
            key={space.id}
            className={`group/space flex h-[30px] items-center rounded-lg pr-1 transition-colors duration-150 max-sm:h-11 ${
              active ? 'bg-list-active text-ink' : 'text-ink-2 hover:bg-list-hover hover:text-ink'
            }`}
          >
            <button
              type="button"
              aria-pressed={active}
              onClick={() => onSelectSpace(active ? null : space.id)}
              className="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-lg px-2.5 text-left text-[13px] max-sm:text-[15px]"
            >
              <span
                className="grid size-[18px] shrink-0 place-items-center"
                title={isSelectedOnly ? 'Selected people' : 'Team space'}
              >
                <SpaceIcon size={13} className={active ? 'text-primary' : 'text-ink-3'} />
              </span>
              <span className={`min-w-0 flex-1 truncate ${active ? 'font-medium text-ink' : ''}`}>
                {space.name}
              </span>
              {typeof count === 'number' && count > 0 ? (
                <span className="shrink-0 text-[11px] text-ink-3 tabular-nums group-hover/space:hidden">
                  {count}
                </span>
              ) : null}
            </button>
            <button
              type="button"
              aria-label={`Manage access for ${space.name}`}
              title={space.can_edit !== false ? 'Manage access' : 'View access'}
              onClick={() => setModal({ mode: 'edit', space })}
              className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/space:opacity-100 [@media(hover:none)]:opacity-100"
            >
              <UserPlus size={12} />
            </button>
            <Link
              to="/new"
              search={{ space: space.id }}
              aria-label={`New thread in ${space.name}`}
              title={`New thread in ${space.name}`}
              className="grid size-6 shrink-0 place-items-center rounded-md text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/space:opacity-100 [@media(hover:none)]:opacity-100"
            >
              <SquarePen size={12} />
            </Link>
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
            onSelectSpace(created.id)
          }}
        />
      ) : null}
    </div>
  )
}
