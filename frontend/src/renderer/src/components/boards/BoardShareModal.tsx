import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { BoardVisibilityFields } from '@/components/boards/BoardVisibilityFields'
import type { PickedMember } from '@/components/cloud/MemberPicker'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { useToast } from '@/components/ui/toast'
import { patchBoard } from '@/lib/api/boards'
import type { Board, BoardVisibility } from '@/lib/api/types'
import { useCloudSession } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

function initialMembers(board: Board): PickedMember[] {
  return (board.members ?? []).map((m) => ({
    user_id: m.user_id,
    email: m.email,
    display_name: m.display_name,
  }))
}

export function BoardShareModal({
  board,
  open,
  onClose,
}: {
  board: Board
  open: boolean
  onClose: () => void
}) {
  const cloud = useCloudSession()
  const queryClient = useQueryClient()
  const toast = useToast()
  const canEdit = board.can_edit !== false
  const [visibility, setVisibility] = useState<BoardVisibility>(board.visibility ?? 'private')
  const [selectedMembers, setSelectedMembers] = useState<PickedMember[]>(() => initialMembers(board))

  const save = useMutation({
    mutationFn: () =>
      patchBoard(board.id, {
        visibility,
        member_user_ids: visibility === 'selected' ? selectedMembers.map((m) => m.user_id) : [],
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.boards })
      toast('Board access updated')
      onClose()
    },
  })

  const excludeUserIds = [board.owner_user_id, cloud.user?.id].filter((v): v is string => Boolean(v))

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="md"
      title={`Share "${board.name}"`}
      description={
        canEdit
          ? 'Choose who in this workspace can view this board.'
          : `Owned by ${board.owner_display_name || board.owner_email || 'a teammate'}.`
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
                disabled={save.isPending}
                onClick={() => save.mutate()}
              >
                {save.isPending ? 'Saving…' : 'Save'}
              </Button>
            ) : null}
          </div>
        </>
      }
    >
      <div className="space-y-4">
        <BoardVisibilityFields
          visibility={visibility}
          onVisibilityChange={setVisibility}
          selectedMembers={selectedMembers}
          onMembersChange={setSelectedMembers}
          excludeUserIds={excludeUserIds}
          canAddTeamMember={cloud.user?.role === 'admin'}
          disabled={!canEdit}
        />
      </div>
    </Modal>
  )
}
