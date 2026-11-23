import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, Trash2, UserPlus, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Input } from '@/components/ui/Input'
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

const ROLE_LABELS: Record<OrgMemberRole, string> = {
  admin: 'Admin',
  member: 'Member',
  viewer: 'Viewer',
}

export function TeamMembersCard({ workspace }: { workspace: CloudWorkspaceSummary }) {
  const cloud = useCloudSession()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isAdmin = (workspace.role || cloud.user?.role) === 'admin'
  const currentUserId = cloud.user?.id
  const currentEmail = (workspace.email || cloud.user?.email || '').toLowerCase()

  const membersQuery = useQuery(orgMembersQuery('', 200))
  const [adding, setAdding] = useState(false)
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [role, setRole] = useState<OrgMemberRole>('member')
  const [search, setSearch] = useState('')

  const invalidateMembers = () => void queryClient.invalidateQueries({ queryKey: keys.cloud })

  const addMutation = useMutation({
    mutationFn: () =>
      createOrgMember({
        email,
        display_name: displayName || undefined,
        role,
      }),
    onSuccess: (created) => {
      invalidateMembers()
      setEmail('')
      setDisplayName('')
      setRole('member')
      setAdding(false)
      toast(`Added ${created.display_name || created.email} to the team`)
    },
  })

  const roleMutation = useMutation({
    mutationFn: ({ id, nextRole }: { id: string; nextRole: OrgMemberRole }) =>
      updateOrgMember(id, { role: nextRole }),
    onSuccess: (updated) => {
      invalidateMembers()
      toast(`Updated ${updated.display_name || updated.email} to ${updated.role}`)
    },
    onError: (err: Error) => toast(err.message, 'danger'),
  })

  const removeMutation = useMutation({
    mutationFn: (id: string) => deleteOrgMember(id),
    onSuccess: () => {
      invalidateMembers()
      toast('Team member removed')
    },
    onError: (err: Error) => toast(err.message, 'danger'),
  })

  const allMembers = useMemo(() => membersQuery.data ?? [], [membersQuery.data])
  const filtered = useMemo(() => filterOrgMembers(allMembers, search), [allMembers, search])

  const isSelf = (member: OrgMember) =>
    Boolean(
      (currentUserId && member.id === currentUserId) ||
        (currentEmail && member.email.toLowerCase() === currentEmail),
    )

  return (
    <div className="space-y-2.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-[14px] font-semibold text-ink">Team members</h2>
          <p className="text-[12px] text-ink-3">
            {isAdmin
              ? 'Add people to share boards and spaces. Pre-added members join automatically when they sign in.'
              : 'Members of this workspace. Admins can add or manage team members.'}
          </p>
        </div>
        {isAdmin ? (
          <Button
            size="sm"
            variant={adding ? 'ghost' : 'secondary'}
            onClick={() => {
              setAdding((prev) => !prev)
              addMutation.reset()
            }}
          >
            {adding ? <X size={13} /> : <UserPlus size={13} />}
            {adding ? 'Cancel' : 'Add member'}
          </Button>
        ) : null}
      </div>

      {adding && isAdmin ? (
        <SettingsCard className="p-3">
          <form
            onSubmit={(event) => {
              event.preventDefault()
              if (email.trim() && !addMutation.isPending) addMutation.mutate()
            }}
            className="space-y-3"
          >
            <div className="grid gap-2.5 sm:grid-cols-2">
              <label className="block">
                <span className="mb-1 block text-[11.5px] font-medium text-ink-2">Email</span>
                <Input
                  type="email"
                  autoFocus
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="teammate@company.com"
                />
              </label>
              <label className="block">
                <span className="mb-1 block text-[11.5px] font-medium text-ink-2">
                  Name <span className="font-normal text-ink-3">(optional)</span>
                </span>
                <Input
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="Alex Rivera"
                />
              </label>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <label className="flex items-center gap-2 text-[12px] text-ink-2">
                <span>Role</span>
                <select
                  aria-label="New member role"
                  value={role}
                  onChange={(e) => setRole(e.target.value as OrgMemberRole)}
                  className="h-7 cursor-pointer rounded-control bg-bg px-2 text-[12px] text-ink outline-none ring-1 ring-border/80"
                >
                  <option value="member">Member</option>
                  <option value="admin">Admin</option>
                  <option value="viewer">Viewer</option>
                </select>
              </label>
              <div className="flex items-center gap-2">
                {addMutation.isError ? (
                  <p role="alert" className="text-[12px] text-danger">
                    {(addMutation.error as Error).message}
                  </p>
                ) : null}
                <Button
                  type="submit"
                  size="sm"
                  variant="primary"
                  disabled={!email.trim() || addMutation.isPending}
                >
                  {addMutation.isPending ? <LoaderCircle size={13} className="animate-spin" /> : null}
                  Add to team
                </Button>
              </div>
            </div>
          </form>
        </SettingsCard>
      ) : null}

      <SettingsCard className="overflow-hidden">
        {allMembers.length > 5 ? (
          <div className="border-b border-border px-3 py-2">
            <Input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Filter team members by name or email…"
              aria-label="Filter team members"
              className="h-8 text-[12.5px]"
            />
          </div>
        ) : null}

        {membersQuery.isPending ? (
          <div className="flex items-center gap-2 px-3 py-3 text-[12.5px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" />
            Loading team members…
          </div>
        ) : membersQuery.isError ? (
          <div className="px-3 py-3 text-[12.5px] text-danger" role="alert">
            {(membersQuery.error as Error).message || 'Could not load team members.'}
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-3 py-3 text-[12.5px] text-ink-3">
            {search.trim() ? 'No matching team members.' : 'No team members yet.'}
          </div>
        ) : (
          filtered.map((member) => {
            const self = isSelf(member)
            const primary = member.display_name || member.email
            const showEmail = member.display_name && member.display_name !== member.email
            const roleValue = (
              member.role === 'admin' || member.role === 'viewer' ? member.role : 'member'
            ) as OrgMemberRole
            return (
              <div
                key={member.id}
                className="flex items-center justify-between gap-3 border-t border-border px-3 py-2.5 first:border-t-0"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <p className="truncate text-[13px] font-medium text-ink">{primary}</p>
                    {self ? (
                      <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-0.5 text-[10.5px] text-ink-3">
                        You
                      </span>
                    ) : null}
                    {member.status === 'invited' ? (
                      <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-0.5 text-[10.5px] text-ink-3">
                        Pending sign-in
                      </span>
                    ) : null}
                  </div>
                  {showEmail ? (
                    <p className="truncate text-[12px] text-ink-3">{member.email}</p>
                  ) : null}
                </div>

                <div className="flex shrink-0 items-center gap-1.5">
                  {isAdmin && !self ? (
                    <>
                      <select
                        aria-label={`Role for ${primary}`}
                        value={roleValue}
                        disabled={roleMutation.isPending}
                        onChange={(e) =>
                          roleMutation.mutate({
                            id: member.id,
                            nextRole: e.target.value as OrgMemberRole,
                          })
                        }
                        className="h-7 cursor-pointer rounded-control bg-bg px-2 text-[12px] text-ink-2 outline-none ring-1 ring-border/70"
                      >
                        <option value="member">Member</option>
                        <option value="admin">Admin</option>
                        <option value="viewer">Viewer</option>
                      </select>
                      <IconButton
                        variant="ghost"
                        size="sm"
                        aria-label={`Remove ${primary}`}
                        title="Remove from team"
                        disabled={removeMutation.isPending}
                        onClick={() => {
                          if (
                            confirm(`Remove ${primary} (${member.email}) from this workspace?`)
                          ) {
                            removeMutation.mutate(member.id)
                          }
                        }}
                      >
                        <Trash2 size={13} />
                      </IconButton>
                    </>
                  ) : (
                    <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[11.5px] font-medium text-ink-2">
                      {ROLE_LABELS[roleValue] ?? member.role}
                    </span>
                  )}
                </div>
              </div>
            )
          })
        )}
      </SettingsCard>
    </div>
  )
}
