import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { useMemo, useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { SearchField } from '@/components/ui/SearchField'
import { useToast } from '@/components/ui/toast'
import {
  createOrgMember,
  deleteOrgMember,
  filterOrgMembers,
  orgMembersQuery,
  updateOrgMember,
  type OrgMember,
  type OrgMemberRole,
} from '@/lib/api/cloudMembers'
import type { CloudWorkspaceSummary } from '@/lib/cloudAccount'
import { useCloudSession } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

const ROLES: { value: OrgMemberRole; label: string }[] = [
  { value: 'admin', label: 'Admin' },
  { value: 'member', label: 'Member' },
  { value: 'viewer', label: 'Viewer' },
]

// A member's role as a quiet menu: the three roles, then removal.
function RoleMenu({ name, role, disabled, onRole, onRemove }: {
  name: string
  role: string
  disabled: boolean
  onRole: (role: OrgMemberRole) => void
  onRemove: () => void
}) {
  const [open, setOpen] = useState(false)
  const pick = (action: () => void) => () => {
    setOpen(false)
    action()
  }
  return (
    <Popover open={open} onClose={() => setOpen(false)} placement="below" align="end" trigger={
      <Button size="sm" aria-label={`Role for ${name}`} aria-haspopup="menu" aria-expanded={open} disabled={disabled} onClick={() => setOpen((value) => !value)}>
        {role}
        <ChevronDown size={12} className="text-ink-3" />
      </Button>
    }>
      <div role="menu" aria-label={`Role for ${name}`}>
        {ROLES.map((option) => (
          <MenuRow key={option.value} role="menuitemradio" selected={option.label === role} onClick={pick(() => onRole(option.value))}>
            {option.label}
          </MenuRow>
        ))}
        <div className="mx-2.5 my-1.5 h-px bg-border" />
        <MenuRow role="menuitem" onClick={pick(onRemove)}>Remove</MenuRow>
      </div>
    </Popover>
  )
}

export function TeamMembersCard({ workspace }: { workspace: CloudWorkspaceSummary }) {
  const cloud = useCloudSession()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isAdmin = (workspace.role || cloud.user?.role) === 'admin'
  const currentUserId = cloud.user?.id
  const currentEmail = (workspace.email || cloud.user?.email || '').toLowerCase()

  const membersQuery = useQuery(orgMembersQuery('', 200))
  const [email, setEmail] = useState('')
  const [search, setSearch] = useState('')

  const invalidateMembers = () =>
    void queryClient.invalidateQueries({ queryKey: keys.cloudMembersRoot })
  const onError = (err: Error) => toast(err.message, 'danger')

  const addMutation = useMutation({
    mutationFn: () => createOrgMember({ email, role: 'member' }),
    onSuccess: () => {
      invalidateMembers()
      setEmail('')
    },
    onError,
  })
  const roleMutation = useMutation({
    mutationFn: ({ id, role }: { id: string; role: OrgMemberRole }) => updateOrgMember(id, { role }),
    onSuccess: invalidateMembers,
    onError,
  })
  const removeMutation = useMutation({
    mutationFn: deleteOrgMember,
    onSuccess: invalidateMembers,
    onError,
  })

  const allMembers = useMemo(() => membersQuery.data ?? [], [membersQuery.data])
  const filtered = useMemo(() => filterOrgMembers(allMembers, search), [allMembers, search])

  const isSelf = (member: OrgMember) =>
    Boolean(
      (currentUserId && member.id === currentUserId) ||
        (currentEmail && member.email.toLowerCase() === currentEmail),
    )

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-[14px] font-semibold text-ink">Members</h2>
        {allMembers.length > 5 ? (
          <SearchField value={search} onChange={setSearch} placeholder="Search…" className="h-8 w-52" />
        ) : null}
      </div>

      {membersQuery.isPending ? null : membersQuery.isError ? (
        <p role="alert" className="text-[13px] text-danger">{membersQuery.error.message || 'Could not load members.'}</p>
      ) : (
        <SettingsCard className="overflow-hidden">
          {filtered.map((member) => {
            const self = isSelf(member)
            const name = member.display_name || member.email
            const role = ROLES.find((option) => option.value === member.role)?.label ?? 'Member'
            const detail = [member.display_name && member.display_name !== member.email ? member.email : '', member.status === 'invited' ? 'Invited' : '']
              .filter(Boolean)
              .join(' · ')
            return (
              <div key={member.id} className="flex items-center gap-3 border-t border-border px-3.5 py-2.5 first:border-t-0">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13.5px] font-medium text-ink">
                    {name}
                    {self ? <span className="font-normal text-ink-3"> (you)</span> : null}
                  </p>
                  {detail ? <p className="truncate text-[12px] text-ink-3">{detail}</p> : null}
                </div>
                {isAdmin && !self ? (
                  <RoleMenu
                    name={name}
                    role={role}
                    disabled={roleMutation.isPending || removeMutation.isPending}
                    onRole={(next) => roleMutation.mutate({ id: member.id, role: next })}
                    onRemove={() => {
                      if (confirm(`Remove ${name} (${member.email}) from this workspace?`)) removeMutation.mutate(member.id)
                    }}
                  />
                ) : (
                  <span className="px-2.5 text-[12px] font-medium text-ink-3">{role}</span>
                )}
              </div>
            )
          })}
          {isAdmin ? (
            <form
              onSubmit={(event) => {
                event.preventDefault()
                addMutation.mutate()
              }}
              className="flex items-center gap-2 border-t border-border px-3.5 py-2.5 first:border-t-0"
            >
              <Input
                type="email"
                required
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="Add by email"
                aria-label="Add a member by email"
                className="bg-transparent px-0 py-1 ring-0 focus:ring-0"
              />
              {email.trim() ? (
                <Button type="submit" size="sm" variant="primary" disabled={addMutation.isPending}>Add</Button>
              ) : null}
            </form>
          ) : null}
        </SettingsCard>
      )}
    </div>
  )
}
