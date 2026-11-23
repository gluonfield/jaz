import { useMutation, useQueryClient } from '@tanstack/react-query'
import { UserCheck, Users } from 'lucide-react'
import { useState } from 'react'
import { MemberPicker, type PickedMember } from '@/components/cloud/MemberPicker'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { useToast } from '@/components/ui/toast'
import {
  createSpace,
  patchSpace,
  type CloudSpace,
  type SpaceVisibility,
} from '@/lib/api/cloudSpaces'
import { useCloudSession } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

const SPACE_VISIBILITY_OPTIONS: {
  value: SpaceVisibility
  label: string
  description: string
  Icon: typeof Users
}[] = [
  {
    value: 'team',
    label: 'Team',
    description: 'Everyone in this workspace can view and chat in this space.',
    Icon: Users,
  },
  {
    value: 'selected',
    label: 'Selected people',
    description: 'Only you and the people you choose.',
    Icon: UserCheck,
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
}: {
  space?: CloudSpace
  onClose: () => void
  onCreated?: (space: CloudSpace) => void
}) {
  const cloud = useCloudSession()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isEdit = Boolean(space)
  const canEdit = !space || space.can_edit !== false
  const [name, setName] = useState(space?.name ?? '')
  const [visibility, setVisibility] = useState<SpaceVisibility>(
    space ? (space.visibility ?? 'selected') : 'team',
  )
  const [selectedMembers, setSelectedMembers] = useState<PickedMember[]>(() =>
    initialSelectedMembers(space),
  )

  const save = useMutation({
    mutationFn: () => {
      const trimmed = name.trim()
      if (!trimmed) throw new Error('Space name is required')
      if (space) {
        return patchSpace(space.id, {
          name: trimmed,
          visibility,
          member_user_ids: visibility === 'selected' ? selectedMembers.map((m) => m.user_id) : [],
        })
      }
      return createSpace({
        name: trimmed,
        visibility,
        ...(visibility === 'selected'
          ? { member_user_ids: selectedMembers.map((m) => m.user_id) }
          : {}),
      })
    },
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudSpaces })
      if (isEdit) {
        toast(`Updated space ${saved.name}`)
      } else {
        onCreated?.(saved)
        toast(`Created space ${saved.name}`)
      }
      onClose()
    },
  })

  const creatorId = space ? space.created_by : cloud.user?.id
  const excludeUserIds = creatorId ? [creatorId] : []
  const activeOption =
    SPACE_VISIBILITY_OPTIONS.find((opt) => opt.value === visibility) ?? SPACE_VISIBILITY_OPTIONS[0]

  return (
    <Modal
      open
      onClose={onClose}
      size="md"
      title={isEdit ? `Space "${space?.name}"` : 'New space'}
      description={
        isEdit
          ? canEdit
            ? 'Manage who in this workspace can access this space.'
            : 'View who has access to this space.'
          : 'A shared channel for team threads and space instructions.'
      }
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {save.isError ? (save.error as Error).message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" onClick={onClose}>
              {canEdit ? 'Cancel' : 'Close'}
            </Button>
            {canEdit ? (
              <Button
                variant="primary"
                size="md"
                disabled={!name.trim() || save.isPending}
                onClick={() => save.mutate()}
              >
                {save.isPending ? 'Saving…' : isEdit ? 'Save' : 'Create space'}
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
            placeholder="Engineering"
          />
        </label>

        <div className="space-y-2.5">
          <div>
            <span className="mb-1.5 block text-[12px] font-medium text-ink-2">Access</span>
            <div
              role="radiogroup"
              aria-label="Space access"
              className="grid grid-cols-2 gap-1.5 rounded-card bg-surface p-1"
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
                    className={`flex cursor-pointer items-center justify-center gap-1.5 rounded-control px-2.5 py-1.5 text-[12.5px] font-medium transition-colors duration-150 ${
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
        </div>
      </div>
    </Modal>
  )
}
