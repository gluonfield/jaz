import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { BoardIllustration } from '@/components/boards/BoardIllustration'
import { BoardVisibilityFields } from '@/components/boards/BoardVisibilityFields'
import type { PickedMember } from '@/components/cloud/MemberPicker'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { createBoard } from '@/lib/api/boards'
import type { BoardVisibility } from '@/lib/api/types'
import { useCloudSession } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

// Create a board, then land on it with the widget picker already open for local
// boards — AddWidgetModal (via ?add=1) owns everything about getting loops onto boards.
export function BoardModal({
  open,
  onClose,
  initialVisibility = 'private',
}: {
  open: boolean
  onClose: () => void
  initialVisibility?: BoardVisibility
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const cloud = useCloudSession()
  const [name, setName] = useState('')
  const [visibility, setVisibility] = useState<BoardVisibility>(initialVisibility)
  const [selectedMembers, setSelectedMembers] = useState<PickedMember[]>([])

  useEffect(() => {
    if (open) {
      setVisibility(initialVisibility)
    }
  }, [open, initialVisibility])

  const close = () => {
    setName('')
    setVisibility(initialVisibility)
    setSelectedMembers([])
    create.reset()
    onClose()
  }

  const create = useMutation({
    mutationFn: () => {
      const boardName = name.trim() || 'Board'
      if (!cloud.active) return createBoard(boardName)
      return createBoard({
        name: boardName,
        visibility,
        ...(visibility === 'selected'
          ? { member_user_ids: selectedMembers.map((m) => m.user_id) }
          : {}),
      })
    },
    onSuccess: (created) => {
      void queryClient.invalidateQueries({ queryKey: keys.boards })
      close()
      void navigate({
        to: '/boards/$boardId',
        params: { boardId: created.id },
        search: cloud.active ? {} : { add: true },
      })
    },
  })

  const excludeUserIds = cloud.user?.id ? [cloud.user.id] : []

  return (
    <Modal
      open={open}
      onClose={close}
      size="lg"
      title="New board"
      description="A tiled board your loops keep up to date."
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {create.isError ? (create.error as Error).message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" onClick={close}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="md"
              disabled={create.isPending}
              onClick={() => create.mutate()}
            >
              {create.isPending ? 'Creating…' : 'Create board'}
            </Button>
          </div>
        </>
      }
    >
      <div className="space-y-5">
        <BoardIllustration />
        <p className="text-[13px] leading-relaxed text-ink-2">
          Assign a loop to a board and every run rewrites its widget — counts, lists, charts —
          using data it just gathered. Drag and resize tiles; your layout always wins.
        </p>
        <label className="block">
          <span className="mb-1.5 block text-[12px] font-medium text-ink-2">Name</span>
          <Input
            type="text"
            value={name}
            autoFocus
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !create.isPending) create.mutate()
            }}
            placeholder="Mission control"
          />
        </label>
        {cloud.active ? (
          <BoardVisibilityFields
            visibility={visibility}
            onVisibilityChange={setVisibility}
            selectedMembers={selectedMembers}
            onMembersChange={setSelectedMembers}
            excludeUserIds={excludeUserIds}
            canAddTeamMember={cloud.user?.role === 'admin'}
          />
        ) : null}
      </div>
    </Modal>
  )
}
