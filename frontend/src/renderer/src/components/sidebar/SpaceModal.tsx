import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Lock, Trash2, UserCheck, Users } from 'lucide-react'
import { useState } from 'react'
import { GitHubRepoField } from '@/components/cloud/GitHubRepoPicker'
import { MemberPicker, type PickedMember } from '@/components/cloud/MemberPicker'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { useToast } from '@/components/ui/toast'
import {
  createSpace,
  deleteSpace,
  patchSpace,
  type CloudSpace,
  type SpaceVisibility,
} from '@/lib/api/cloudSpaces'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'

const SPACE_VISIBILITY_OPTIONS: {
  value: SpaceVisibility
  label: string
  description: string
  Icon: typeof Users
}[] = [
  {
    value: 'private',
    label: 'Private',
    description: 'Only you can view sources, instructions, and chats in this project.',
    Icon: Lock,
  },
  {
    value: 'selected',
    label: 'Selected',
    description: 'Only you and the workspace members you choose.',
    Icon: UserCheck,
  },
  {
    value: 'team',
    label: 'Team',
    description: 'Everyone in this workspace can view and chat in this project.',
    Icon: Users,
  },
]

function initialSelectedMembers(space?: CloudSpace): PickedMember[] {
  if (!space?.members) return []
  return space.members
    .filter((m): m is typeof m & { user_id: string } => Boolean(m.user_id && m.user_id !== space.created_by))
    .map((m) => ({
      user_id: m.user_id,
      email: m.email ?? '',
      display_name: m.display_name ?? m.email ?? m.user_id,
    }))
}

export function SpaceModal({
  space,
  onClose,
  onCreated,
  onDeleted,
}: {
  space?: CloudSpace
  onClose: () => void
  onCreated?: (space: CloudSpace) => void
  onDeleted?: (spaceId: string) => void
}) {
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isEdit = Boolean(space)
  const canEdit = !isViewer && (!space || space.can_edit !== false)
  const [name, setName] = useState(space?.name ?? '')
  const [visibility, setVisibility] = useState<SpaceVisibility>(
    space ? (space.visibility ?? 'selected') : 'private',
  )
  const [selectedMembers, setSelectedMembers] = useState<PickedMember[]>(() =>
    initialSelectedMembers(space),
  )
  const [defaultGitRepo, setDefaultGitRepo] = useState(space?.default_git_repo ?? '')
  const [defaultGitBranch, setDefaultGitBranch] = useState(space?.default_git_branch ?? '')
  const [confirmDelete, setConfirmDelete] = useState(false)

  const save = useMutation({
    mutationFn: () => {
      const trimmed = name.trim()
      if (!trimmed) throw new Error('Project name is required')
      const gitRepo = defaultGitRepo.trim()
      const gitBranch = gitRepo ? defaultGitBranch.trim() : ''
      if (space) {
        return patchSpace(space.id, {
          name: trimmed,
          visibility,
          member_user_ids: visibility === 'selected' ? selectedMembers.map((m) => m.user_id) : [],
          default_git_repo: gitRepo,
          default_git_branch: gitBranch,
        })
      }
      return createSpace({
        name: trimmed,
        visibility,
        ...(visibility === 'selected'
          ? { member_user_ids: selectedMembers.map((m) => m.user_id) }
          : {}),
        ...(gitRepo ? { default_git_repo: gitRepo } : {}),
        ...(gitBranch ? { default_git_branch: gitBranch } : {}),
      })
    },
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpaces })
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpace(saved.id) })
      if (isEdit) {
        toast(`Updated project ${saved.name}`)
      } else {
        onCreated?.(saved)
        toast(`Created project ${saved.name}`)
      }
      onClose()
    },
  })

  const remove = useMutation({
    mutationFn: () => {
      if (!space) throw new Error('Project not found')
      return deleteSpace(space.id)
    },
    onSuccess: () => {
      if (!space) return
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpaces })
      void queryClient.removeQueries({ queryKey: keys.cloudSpace(space.id) })
      invalidateSessionLists(queryClient)
      toast(`Deleted project ${space.name}`)
      onDeleted?.(space.id)
      onClose()
    },
  })

  const creatorId = space ? space.created_by : cloud.user?.id
  const excludeUserIds = creatorId ? [creatorId] : []
  const activeOption =
    SPACE_VISIBILITY_OPTIONS.find((opt) => opt.value === visibility) ?? SPACE_VISIBILITY_OPTIONS[0]
  const errorMessage = save.isError
    ? (save.error as Error).message
    : remove.isError
      ? (remove.error as Error).message
      : ''

  return (
    <Modal
      open
      onClose={onClose}
      size="md"
      title={isEdit ? `Project "${space?.name}"` : 'New project'}
      description={
        isEdit
          ? canEdit
            ? 'Rename this project or manage who in this workspace can access it.'
            : 'View who has access to this project.'
          : 'Organize related chats, source files, and custom instructions in one project.'
      }
      footer={
        <>
          <div className="flex min-w-0 items-center gap-2">
            {isEdit && canEdit ? (
              confirmDelete ? (
                <Button
                  variant="danger"
                  size="sm"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate()}
                >
                  <Trash2 size={13} />
                  {remove.isPending ? 'Deleting…' : 'Confirm delete'}
                </Button>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={save.isPending}
                  onClick={() => setConfirmDelete(true)}
                  className="text-danger hover:text-danger"
                >
                  <Trash2 size={13} />
                  Delete
                </Button>
              )
            ) : null}
            {errorMessage ? (
              <p className="truncate text-[12px] text-danger" role="alert">
                {errorMessage}
              </p>
            ) : confirmDelete ? (
              <p className="truncate text-[11.5px] text-ink-3">Chats stay private; sources are removed.</p>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" onClick={onClose}>
              {canEdit ? 'Cancel' : 'Close'}
            </Button>
            {canEdit ? (
              <Button
                variant="primary"
                size="md"
                disabled={!name.trim() || save.isPending || remove.isPending}
                onClick={() => save.mutate()}
              >
                {save.isPending ? 'Saving…' : isEdit ? 'Save' : 'Create project'}
              </Button>
            ) : null}
          </div>
        </>
      }
    >
      <div className="space-y-4">
        <label className="block">
          <span className="mb-1.5 block text-[12px] font-medium text-ink-2">Name</span>
          <Input
            type="text"
            value={name}
            autoFocus={!isEdit}
            disabled={!canEdit}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canEdit && name.trim() && !save.isPending) save.mutate()
            }}
            placeholder="Q4 Product Launch"
          />
        </label>

        <div className="space-y-2.5">
          <div>
            <span className="mb-1.5 block text-[12px] font-medium text-ink-2">Access</span>
            <div
              role="radiogroup"
              aria-label="Project access"
              className="grid grid-cols-3 gap-1 rounded-card bg-surface p-1"
            >
              {SPACE_VISIBILITY_OPTIONS.map(({ value, label, Icon }) => {
                const active = visibility === value
                return (
                  <button
                    key={value}
                    type="button"
                    role="radio"
                    aria-checked={active}
                    disabled={!canEdit}
                    onClick={() => setVisibility(value)}
                    className={`flex cursor-pointer items-center justify-center gap-1.5 rounded-control px-2 py-1.5 text-[12.5px] font-medium transition-colors duration-150 ${
                      active
                        ? 'bg-bg text-ink shadow-xs ring-1 ring-border/70'
                        : 'text-ink-3 hover:text-ink'
                    } ${!canEdit ? 'cursor-default opacity-70' : ''}`}
                  >
                    <Icon size={13} className={active ? 'text-primary' : 'text-ink-3'} />
                    <span className="truncate">{label}</span>
                  </button>
                )
              })}
            </div>
            <p className="mt-1.5 text-[12px] text-ink-3">{activeOption.description}</p>
          </div>

          {visibility === 'selected' ? (
            <div>
              <span className="mb-1.5 block text-[12px] font-medium text-ink-2">People</span>
              <MemberPicker
                selected={selectedMembers}
                onChange={setSelectedMembers}
                excludeUserIds={excludeUserIds}
                canAddTeamMember={cloud.user?.role === 'admin'}
                disabled={!canEdit}
              />
            </div>
          ) : null}

          <GitHubRepoField
            repo={defaultGitRepo}
            baseBranch={defaultGitBranch}
            disabled={!canEdit}
            onChange={({ repo, baseBranch }) => {
              setDefaultGitRepo(repo)
              setDefaultGitBranch(baseBranch)
            }}
          />
        </div>
      </div>
    </Modal>
  )
}
