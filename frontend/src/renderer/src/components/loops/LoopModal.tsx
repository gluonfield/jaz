import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ArrowLeft, LayoutTemplate } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { boardsQuery } from '@/lib/api/boards'
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
import { LoopTemplateGallery } from './LoopTemplateGallery'
import { draftFromTemplate, type LoopTemplate } from './loopTemplates'

// One modal for both create (no `loop`) and edit (`loop` provided). Edit never
// happens inline on the detail page — it always opens here. Creating shows the
// form directly; "Examples" swaps in a template gallery that fills the form.
export function LoopModal({
  open,
  onClose,
  loop,
  boardIds,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  loop?: Loop
  // Current board assignments when editing (from the loop detail response).
  boardIds?: string[]
  // When set, creating stays in place (no navigation) and reports the loop —
  // the board scrolls its new tile into view instead.
  onCreated?: (loop: Loop) => void
}) {
  const isEdit = !!loop
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const settingsQuery = useQuery(agentSettingsQuery)
  const boards = useQuery({ ...boardsQuery, enabled: open && !isEdit })
  const [draft, setDraft] = useState<LoopDraft | null>(null)
  // Create only: when true the body shows the examples gallery instead of the form.
  const [browsing, setBrowsing] = useState(false)
  const createBoardIds = boards.data?.map((board) => board.id) ?? []
  const createBoardsLoading = !isEdit && boards.isPending
  const current = draft ?? (loop ? loopDraftFromLoop(loop, boardIds) : emptyLoopDraft(createBoardIds))

  const pickTemplate = (template: LoopTemplate) => {
    setDraft(draftFromTemplate(template, createBoardIds))
    setBrowsing(false)
  }

  const save = useMutation<Loop, Error, { run: boolean }>({
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
    setBrowsing(false)
    save.reset()
    onClose()
  }

  const canSubmit = canSaveLoop(current) && !save.isPending && !createBoardsLoading
  const description = browsing
    ? 'Pick an example to fill the form.'
    : 'A prompt that runs on a schedule, each run in its own thread.'

  return (
    <Modal
      open={open}
      onClose={close}
      size="md"
      title={browsing ? 'Examples' : isEdit ? 'Edit loop' : 'New loop'}
      description={description}
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {save.isError ? save.error.message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            {browsing ? (
              <Button variant="ghost" size="md" onClick={() => setBrowsing(false)}>
                <ArrowLeft size={14} />
                Back
              </Button>
            ) : null}
            <Button variant="ghost" size="md" onClick={close}>
              Cancel
            </Button>
            {browsing ? null : isEdit ? (
              <Button
                variant="primary"
                size="md"
                disabled={!canSubmit}
                onClick={() => save.mutate({ run: false })}
              >
                {save.isPending ? 'Saving…' : 'Save changes'}
              </Button>
            ) : (
              <>
                <Button
                  variant="secondary"
                  size="md"
                  disabled={!canSubmit}
                  onClick={() => save.mutate({ run: false })}
                >
                  {save.isPending && !save.variables?.run ? 'Creating…' : 'Create'}
                </Button>
                <Button
                  variant="primary"
                  size="md"
                  disabled={!canSubmit}
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
      {/* Gallery and form are swapped, not co-mounted: the prompt seeds its
          value from the draft at mount, so a template fill only shows once the
          form remounts. Don't switch this to a hidden/always-mounted toggle. */}
      {browsing ? (
        <LoopTemplateGallery onPick={pickTemplate} />
      ) : (
        <div className="space-y-4">
          {!isEdit ? (
            <div className="flex justify-end">
              <Button variant="secondary" size="sm" onClick={() => setBrowsing(true)}>
                <LayoutTemplate size={14} />
                Examples
              </Button>
            </div>
          ) : null}
          <LoopForm
            draft={current}
            disabled={save.isPending || createBoardsLoading}
            autoFocusPrompt={!isEdit}
            onChange={setDraft}
          />
        </div>
      )}
    </Modal>
  )
}
