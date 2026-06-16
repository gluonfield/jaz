import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { createLoop, runLoopNow, updateLoop } from '@/lib/api/loops'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { Loop } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import {
  type LoopDraft,
  LoopForm,
  canSaveLoop,
  emptyLoopDraft,
  loopDraftFromLoop,
  loopDraftToInput,
} from './LoopForm'

// One modal for both create (no `loop`) and edit (`loop` provided). Edit never
// happens inline on the detail page — it always opens here.
export function LoopModal({
  open,
  onClose,
  loop,
  boardIds,
  initialBoardIds,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  loop?: Loop
  // Current board assignments when editing (from the loop detail response).
  boardIds?: string[]
  // Preselected boards when creating (e.g. "New widget" from a board).
  initialBoardIds?: string[]
  // When set, creating stays in place (no navigation) and reports the loop —
  // the board scrolls its new tile into view instead.
  onCreated?: (loop: Loop) => void
}) {
  const isEdit = !!loop
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const settingsQuery = useQuery(agentSettingsQuery)
  const [draft, setDraft] = useState<LoopDraft | null>(null)
  const current = draft ?? (loop ? loopDraftFromLoop(loop, boardIds) : emptyLoopDraft(initialBoardIds))

  const save = useMutation({
    mutationFn: ({ run: _run }: { run: boolean }) =>
      isEdit
        ? updateLoop(loop.id, loopDraftToInput(current, settingsQuery.data))
        : createLoop(loopDraftToInput(current, settingsQuery.data)),
    onSuccess: (saved, { run }) => {
      if (!isEdit && run) void runLoopNow(saved.id).catch(() => {})
      queryClient.invalidateQueries({ queryKey: keys.loops })
      queryClient.invalidateQueries({ queryKey: keys.boards })
      if (isEdit) queryClient.invalidateQueries({ queryKey: keys.loopDetail(loop.id) })
      close()
      if (!isEdit) {
        if (onCreated) onCreated(saved)
        // Board OS windows never navigate; the new loop opens in the main app.
        else if (window.jaz?.windowKind === 'board') window.jaz.openInMain(`/loops/${saved.id}`)
        else navigate({ to: '/loops/$loopId', params: { loopId: saved.id } })
      }
    },
  })

  const close = () => {
    setDraft(null)
    save.reset()
    onClose()
  }

  return (
    <Modal
      open={open}
      onClose={close}
      size="md"
      title={isEdit ? 'Edit loop' : 'New loop'}
      description="A prompt that runs on a schedule, each run in its own thread."
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {save.isError ? (save.error as Error).message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" onClick={close}>
              Cancel
            </Button>
            {isEdit ? (
              <Button
                variant="primary"
                size="md"
                disabled={!canSaveLoop(current) || save.isPending}
                onClick={() => save.mutate({ run: false })}
              >
                {save.isPending ? 'Saving…' : 'Save changes'}
              </Button>
            ) : (
              <>
                <Button
                  variant="secondary"
                  size="md"
                  disabled={!canSaveLoop(current) || save.isPending}
                  onClick={() => save.mutate({ run: false })}
                >
                  {save.isPending && !save.variables?.run ? 'Creating…' : 'Create'}
                </Button>
                <Button
                  variant="primary"
                  size="md"
                  disabled={!canSaveLoop(current) || save.isPending}
                  onClick={() => save.mutate({ run: true })}
                >
                  {save.isPending && save.variables?.run ? 'Creating…' : 'Create & Run'}
                </Button>
              </>
            )}
          </div>
        </>
      }
    >
      <LoopForm
        draft={current}
        disabled={save.isPending}
        onChange={setDraft}
      />
    </Modal>
  )
}
