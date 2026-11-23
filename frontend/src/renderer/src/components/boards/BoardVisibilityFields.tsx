import { Lock, UserCheck, Users } from 'lucide-react'
import { MemberPicker, type PickedMember } from '@/components/cloud/MemberPicker'
import type { Board, BoardVisibility } from '@/lib/api/types'

const VISIBILITY_OPTIONS: {
  value: BoardVisibility
  label: string
  description: string
  Icon: typeof Lock
}[] = [
  {
    value: 'private',
    label: 'Private',
    description: 'Only you can view this board.',
    Icon: Lock,
  },
  {
    value: 'team',
    label: 'Team',
    description: 'Everyone in this workspace can view this board.',
    Icon: Users,
  },
  {
    value: 'selected',
    label: 'Selected people',
    description: 'Only you and the people you choose.',
    Icon: UserCheck,
  },
]

export function BoardVisibilityFields({
  visibility,
  onVisibilityChange,
  selectedMembers,
  onMembersChange,
  excludeUserIds = [],
  canAddTeamMember = false,
  disabled = false,
}: {
  visibility: BoardVisibility
  onVisibilityChange: (next: BoardVisibility) => void
  selectedMembers: PickedMember[]
  onMembersChange: (next: PickedMember[]) => void
  excludeUserIds?: string[]
  canAddTeamMember?: boolean
  disabled?: boolean
}) {
  const activeOption =
    VISIBILITY_OPTIONS.find((opt) => opt.value === visibility) ?? VISIBILITY_OPTIONS[0]

  return (
    <div className="space-y-2.5">
      <div>
        <span className="mb-1.5 block text-[12px] font-medium text-ink-2">Visibility</span>
        <div
          role="radiogroup"
          aria-label="Board visibility"
          className="grid grid-cols-3 gap-1.5 rounded-card bg-surface p-1"
        >
          {VISIBILITY_OPTIONS.map(({ value, label, Icon }) => {
            const active = visibility === value
            return (
              <button
                key={value}
                type="button"
                role="radio"
                aria-checked={active}
                disabled={disabled}
                onClick={() => onVisibilityChange(value)}
                className={`flex cursor-pointer items-center justify-center gap-1.5 rounded-control px-2.5 py-1.5 text-[12.5px] font-medium transition-colors duration-150 ${
                  active
                    ? 'bg-bg text-ink shadow-xs ring-1 ring-border/70'
                    : 'text-ink-3 hover:text-ink'
                } ${disabled ? 'cursor-default opacity-70' : ''}`}
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
            onChange={onMembersChange}
            excludeUserIds={excludeUserIds}
            canAddTeamMember={canAddTeamMember}
            disabled={disabled}
          />
        </div>
      ) : null}
    </div>
  )
}

export function boardVisibilityIcon(visibility?: BoardVisibility): typeof Lock {
  return (VISIBILITY_OPTIONS.find((opt) => opt.value === visibility) ?? VISIBILITY_OPTIONS[0]).Icon
}

export function boardVisibilityLabel(board: Pick<Board, 'visibility' | 'members'>): string {
  if (board.visibility === 'team') return 'Team'
  if (board.visibility === 'selected') {
    const count = board.members?.length ?? 0
    return count > 0 ? `Selected · ${count}` : 'Selected'
  }
  return 'Private'
}

export function BoardVisibilityBadge({
  board,
  className = '',
}: {
  board: Pick<Board, 'visibility' | 'members'>
  className?: string
}) {
  const Icon = boardVisibilityIcon(board.visibility)
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[11px] font-medium text-ink-2 ${className}`}
    >
      <Icon size={11} className="shrink-0 text-ink-3" />
      <span>{boardVisibilityLabel(board)}</span>
    </span>
  )
}
