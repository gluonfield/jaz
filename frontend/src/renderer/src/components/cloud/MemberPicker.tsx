import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, UserPlus, X } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { useOrgMemberSearch } from '@/components/cloud/useOrgMemberSearch'
import {
  createOrgMember,
  filterOrgMembers,
  looksLikeEmail,
  type OrgMember,
} from '@/lib/api/cloudMembers'
import { keys } from '@/lib/query/keys'

export interface PickedMember {
  user_id: string
  email: string
  display_name: string
}

export function MemberPicker({
  selected,
  onChange,
  excludeUserIds = [],
  canAddTeamMember = false,
  placeholder = 'Type a name or email…',
  disabled = false,
}: {
  selected: PickedMember[]
  onChange: (next: PickedMember[]) => void
  excludeUserIds?: string[]
  canAddTeamMember?: boolean
  placeholder?: string
  disabled?: boolean
}) {
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLInputElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)

  const { members: allKnown, isSearching } = useOrgMemberSearch(query, !disabled)

  const excludeSet = useMemo(() => {
    const set = new Set<string>(excludeUserIds)
    for (const item of selected) set.add(item.user_id)
    return set
  }, [excludeUserIds, selected])

  const matches = useMemo(
    () => filterOrgMembers(allKnown, query, excludeSet).slice(0, 8),
    [allKnown, query, excludeSet],
  )

  const trimmed = query.trim()
  const lowerTrimmed = trimmed.toLowerCase()
  const exactEmailExists = useMemo(() => {
    if (!lowerTrimmed) return false
    if (selected.some((item) => item.email.toLowerCase() === lowerTrimmed)) return true
    return allKnown.some((member) => member.email.toLowerCase() === lowerTrimmed)
  }, [allKnown, lowerTrimmed, selected])

  const canQuickAdd = canAddTeamMember && looksLikeEmail(trimmed) && !exactEmailExists

  const appendMember = (member: PickedMember) => {
    onChange([...selected, member])
    setQuery('')
    setActiveIndex(0)
    inputRef.current?.focus()
  }

  const quickAdd = useMutation({
    mutationFn: (email: string) => createOrgMember({ email, role: 'member' }),
    onSuccess: (created) => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudMembersRoot })
      appendMember({ user_id: created.id, email: created.email, display_name: created.display_name })
    },
  })

  const selectMember = (member: OrgMember) =>
    appendMember({ user_id: member.id, email: member.email, display_name: member.display_name })

  const removeMember = (userId: string) => {
    onChange(selected.filter((item) => item.user_id !== userId))
  }

  const totalOptions = matches.length + (canQuickAdd ? 1 : 0)
  const clampedActiveIndex = totalOptions > 0 ? Math.min(activeIndex, totalOptions - 1) : 0
  const showDropdown =
    open && !disabled && (matches.length > 0 || canQuickAdd || isSearching || trimmed.length > 0)

  return (
    <div
      ref={containerRef}
      className="relative"
      onBlur={(event) => {
        if (!containerRef.current?.contains(event.relatedTarget as Node | null)) {
          setOpen(false)
        }
      }}
    >
      <div
        onClick={() => {
          if (!disabled) inputRef.current?.focus()
        }}
        className={`flex min-h-[38px] flex-wrap items-center gap-1.5 rounded-control bg-bg px-2.5 py-1.5 ring-1 ring-border/80 transition-shadow focus-within:ring-2 focus-within:ring-primary/40 ${
          disabled ? 'opacity-60' : 'cursor-text'
        }`}
      >
        {selected.map((person) => {
          const label = person.display_name || person.email
          return (
            <span
              key={person.user_id}
              title={person.email}
              className="inline-flex max-w-full items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[12px] text-ink"
            >
              <span className="truncate">{label}</span>
              {!disabled ? (
                <button
                  type="button"
                  aria-label={`Remove ${label}`}
                  onClick={(event) => {
                    event.stopPropagation()
                    removeMember(person.user_id)
                  }}
                  className="grid size-3.5 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors hover:bg-bg hover:text-ink"
                >
                  <X size={10} />
                </button>
              ) : null}
            </span>
          )
        })}
        <input
          ref={inputRef}
          type="text"
          value={query}
          disabled={disabled}
          placeholder={selected.length === 0 ? placeholder : 'Add another person…'}
          aria-label="Search team members by name or email"
          aria-expanded={showDropdown}
          aria-autocomplete="list"
          onFocus={() => setOpen(true)}
          onChange={(event) => {
            setQuery(event.target.value)
            setActiveIndex(0)
            setOpen(true)
            if (quickAdd.isError) quickAdd.reset()
          }}
          onKeyDown={(event) => {
            if (event.key === 'ArrowDown') {
              if (totalOptions > 0) {
                event.preventDefault()
                setOpen(true)
                setActiveIndex((prev) => (prev + 1) % totalOptions)
              }
            } else if (event.key === 'ArrowUp') {
              if (totalOptions > 0) {
                event.preventDefault()
                setOpen(true)
                setActiveIndex((prev) => (prev - 1 + totalOptions) % totalOptions)
              }
            } else if (event.key === 'Enter' && open) {
              if (clampedActiveIndex < matches.length) {
                event.preventDefault()
                selectMember(matches[clampedActiveIndex])
              } else if (canQuickAdd && !quickAdd.isPending) {
                event.preventDefault()
                quickAdd.mutate(trimmed)
              }
            } else if (event.key === 'Escape') {
              setOpen(false)
            } else if (event.key === 'Backspace' && !query && selected.length > 0) {
              removeMember(selected[selected.length - 1].user_id)
            }
          }}
          className="min-w-[140px] flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-ink-3"
        />
      </div>

      {quickAdd.isError ? (
        <p role="alert" className="mt-1 text-[12px] text-danger">
          {(quickAdd.error as Error).message}
        </p>
      ) : null}

      {showDropdown ? (
        <div
          role="listbox"
          className="absolute left-0 right-0 z-30 mt-1 max-h-56 overflow-y-auto rounded-card bg-bg p-1 shadow-lg ring-1 ring-border"
        >
          {matches.map((member, index) => {
            const active = index === clampedActiveIndex
            const primary = member.display_name || member.email
            const showEmail = member.display_name && member.display_name !== member.email
            return (
              <button
                key={member.id}
                type="button"
                role="option"
                aria-selected={active}
                onMouseDown={(event) => event.preventDefault()}
                onMouseEnter={() => setActiveIndex(index)}
                onClick={() => selectMember(member)}
                className={`flex w-full cursor-pointer items-center justify-between gap-2 rounded-control px-2.5 py-1.5 text-left transition-colors ${
                  active ? 'bg-list-hover text-ink' : 'text-ink-2 hover:bg-list-hover hover:text-ink'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] font-medium text-ink">{primary}</span>
                  {showEmail ? (
                    <span className="block truncate text-[11.5px] text-ink-3">{member.email}</span>
                  ) : null}
                </span>
                {member.status === 'invited' ? (
                  <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-0.5 text-[10.5px] text-ink-3">
                    Invited
                  </span>
                ) : null}
              </button>
            )
          })}

          {canQuickAdd ? (
            <button
              type="button"
              disabled={quickAdd.isPending}
              onMouseDown={(event) => event.preventDefault()}
              onMouseEnter={() => setActiveIndex(matches.length)}
              onClick={() => quickAdd.mutate(trimmed)}
              className={`flex w-full cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-left text-[12.5px] font-medium text-primary transition-colors ${
                clampedActiveIndex === matches.length ? 'bg-list-hover' : 'hover:bg-list-hover'
              }`}
            >
              {quickAdd.isPending ? (
                <LoaderCircle size={13} className="shrink-0 animate-spin" />
              ) : (
                <UserPlus size={13} className="shrink-0" />
              )}
              <span className="truncate">Add {trimmed} to team &amp; select</span>
            </button>
          ) : null}

          {matches.length === 0 && !canQuickAdd ? (
            <p className="px-2.5 py-2 text-[12px] text-ink-3">
              {isSearching
                ? 'Searching team…'
                : trimmed
                  ? 'No matching team members found.'
                  : 'No other team members yet.'}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
