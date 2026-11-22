import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, SquarePen, UserPlus, Users, X } from 'lucide-react'
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { useToast } from '@/components/ui/toast'
import { addSpaceMember, createSpace, spacesQuery, type CloudSpace } from '@/lib/api/cloud'
import { keys } from '@/lib/query/keys'

export function SidebarSpaces({
  selectedSpaceId,
  onSelectSpace,
}: {
  selectedSpaceId: string | null
  onSelectSpace: (spaceId: string | null) => void
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const spaces = useQuery(spacesQuery)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [memberTarget, setMemberTarget] = useState<string | null>(null)
  const [memberInput, setMemberInput] = useState('')
  const [memberRole, setMemberRole] = useState<'member' | 'admin'>('member')

  const create = useMutation({
    mutationFn: (name: string) => createSpace({ name }),
    onSuccess: (created) => {
      queryClient.setQueryData<CloudSpace[]>(keys.cloudSpaces, (prev) => [...(prev ?? []), created])
      setNewName('')
      setCreating(false)
      onSelectSpace(created.id)
      toast(`Created space ${created.name}`)
    },
    onError: (error: Error) => toast(error.message, 'danger'),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: keys.cloudSpaces }),
  })

  const invite = useMutation({
    mutationFn: ({ spaceId, identity, role }: { spaceId: string; identity: string; role: string }) => {
      const trimmed = identity.trim()
      return addSpaceMember(
        spaceId,
        trimmed.includes('@') ? { email: trimmed, role } : { user_id: trimmed, role },
      )
    },
    onSuccess: () => {
      setMemberInput('')
      setMemberTarget(null)
      toast('Member added to space')
    },
    onError: (error: Error) => toast(error.message, 'danger'),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: keys.cloudSpaces }),
  })

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
            onClick={() => setCreating((value) => !value)}
            className="grid size-6 cursor-pointer place-items-center rounded-md text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink"
          >
            {creating ? <X size={13} /> : <Plus size={13} />}
          </button>
        </div>
      </div>

      {creating ? (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (newName.trim() && !create.isPending) create.mutate(newName.trim())
          }}
          className="flex items-center gap-1.5 px-2 pb-1.5"
        >
          <Input
            autoFocus
            value={newName}
            onChange={(event) => setNewName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Escape') {
                setCreating(false)
                setNewName('')
              }
            }}
            placeholder="Space name"
            aria-label="New space name"
            className="h-7 flex-1 text-[12.5px]"
          />
          <Button type="submit" size="sm" variant="primary" disabled={!newName.trim() || create.isPending}>
            Add
          </Button>
        </form>
      ) : null}

      {list.length === 0 && !spaces.isPending && !creating ? (
        <p className="px-2.5 py-1 text-[12px] text-ink-3">No shared spaces yet</p>
      ) : null}

      {list.map((space) => {
        const active = selectedSpaceId === space.id
        const addingMember = memberTarget === space.id
        const count = space.member_count ?? space.members?.length
        return (
          <div key={space.id} className="flex flex-col">
            <div
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
                <span className="grid size-[18px] shrink-0 place-items-center">
                  <Users size={14} className={active ? 'text-primary' : 'text-ink-3'} />
                </span>
                <span className={`min-w-0 flex-1 truncate ${active ? 'font-medium text-ink' : ''}`}>{space.name}</span>
                {typeof count === 'number' && count > 0 ? (
                  <span className="shrink-0 text-[11px] text-ink-3 tabular-nums group-hover/space:hidden">
                    {count}
                  </span>
                ) : null}
              </button>
              <button
                type="button"
                aria-label={`Add member to ${space.name}`}
                title="Add member"
                onClick={() => {
                  setMemberTarget(addingMember ? null : space.id)
                  setMemberInput('')
                }}
                className={`grid size-6 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/space:opacity-100 [@media(hover:none)]:opacity-100 ${
                  addingMember ? 'opacity-100 text-ink' : 'opacity-0'
                }`}
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

            {addingMember ? (
              <form
                onSubmit={(event) => {
                  event.preventDefault()
                  if (memberInput.trim() && !invite.isPending) {
                    invite.mutate({ spaceId: space.id, identity: memberInput, role: memberRole })
                  }
                }}
                className="mt-1 mb-1.5 flex flex-col gap-1.5 rounded-control bg-surface p-2"
              >
                <Input
                  autoFocus
                  value={memberInput}
                  onChange={(event) => setMemberInput(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Escape') {
                      setMemberTarget(null)
                      setMemberInput('')
                    }
                  }}
                  placeholder="Email or user ID"
                  aria-label={`Member email or user ID for ${space.name}`}
                  className="h-7 text-[12px]"
                />
                <div className="flex items-center justify-between gap-1.5">
                  <select
                    aria-label="Member role"
                    value={memberRole}
                    onChange={(event) => setMemberRole(event.target.value as 'member' | 'admin')}
                    className="h-6 cursor-pointer rounded-full bg-bg px-2 text-[11px] text-ink-2 outline-none ring-1 ring-border/70"
                  >
                    <option value="member">Member</option>
                    <option value="admin">Admin</option>
                  </select>
                  <div className="flex items-center gap-1">
                    <Button size="sm" variant="ghost" onClick={() => setMemberTarget(null)}>
                      Cancel
                    </Button>
                    <Button size="sm" variant="primary" type="submit" disabled={!memberInput.trim() || invite.isPending}>
                      Invite
                    </Button>
                  </div>
                </div>
              </form>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}
