import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Repeat } from 'lucide-react'
import { useState } from 'react'
import { Modal } from '@/components/ui/Modal'
import { createLoop, updateLoop } from '@/lib/api/loops'
import { acpAgentsQuery } from '@/lib/api/sessions'
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
}: {
  open: boolean
  onClose: () => void
  loop?: Loop
}) {
  const isEdit = !!loop
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: agents = [] } = useQuery(acpAgentsQuery)
  const [draft, setDraft] = useState<LoopDraft | null>(null)
  const current = draft ?? (loop ? loopDraftFromLoop(loop) : emptyLoopDraft(agents))

  const save = useMutation({
    mutationFn: () =>
      isEdit ? updateLoop(loop.id, loopDraftToInput(current)) : createLoop(loopDraftToInput(current)),
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: keys.loops })
      if (isEdit) queryClient.invalidateQueries({ queryKey: keys.loopDetail(loop.id) })
      close()
      if (!isEdit) navigate({ to: '/loops/$loopId', params: { loopId: saved.id } })
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
      icon={<Repeat size={16} />}
      title={isEdit ? 'Edit loop' : 'New loop'}
      description="A prompt that runs on a schedule, each run in its own thread."
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {save.isError ? (save.error as Error).message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <button
              type="button"
              onClick={close}
              className="rounded-control px-3 py-1.5 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={!canSaveLoop(current) || save.isPending}
              onClick={() => save.mutate()}
              className="rounded-control bg-primary px-3 py-1.5 text-[13px] font-medium text-on-primary transition-colors duration-150 hover:bg-primary-strong disabled:cursor-default disabled:opacity-50"
            >
              {save.isPending ? 'Saving…' : isEdit ? 'Save changes' : 'Create loop'}
            </button>
          </div>
        </>
      }
    >
      <LoopForm draft={current} agents={agents} disabled={save.isPending} onChange={setDraft} />
    </Modal>
  )
}
