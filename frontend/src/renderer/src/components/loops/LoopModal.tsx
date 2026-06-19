import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
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
// happens inline on the detail page — it always opens here. Creating opens on a
// template gallery first; picking one (or "from scratch") reveals the form.
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
  const createBoardIds = boards.data?.map((board) => board.id) ?? []
  const createBoardsLoading = !isEdit && boards.isPending
  // Create starts on the gallery: the form only appears once a draft exists.
  const editDraft = loop ? loopDraftFromLoop(loop, boardIds) : null
  const current = draft ?? editDraft
  const onForm = isEdit || draft !== null

  const startFromTemplate = (template: LoopTemplate) =>
    setDraft(draftFromTemplate(template, createBoardIds))
  const startBlank = () => setDraft(emptyLoopDraft(createBoardIds))

  const save = useMutation<Loop, Error, { run: boolean }>({
    mutationFn: ({ run: _run }: { run: boolean }) => {
      if (!current) throw new Error('No loop to save')
      return isEdit
        ? updateLoop(loop.id, loopDraftToInput(current, settingsQuery.data))
        : createLoop(loopDraftToInput(current, settingsQuery.data))
    },
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

  const canSubmit = !!current && canSaveLoop(current) && !save.isPending && !createBoardsLoading
  const description = onForm
    ? 'A prompt that runs on a schedule, each run in its own thread.'
    : 'Start from a template, or from scratch.'

  return (
    <Modal
      open={open}
      onClose={close}
      size="md"
      title={isEdit ? 'Edit loop' : 'New loop'}
      description={description}
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {save.isError ? save.error.message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            {onForm && !isEdit ? (
              <Button variant="ghost" size="md" onClick={() => setDraft(null)}>
                <ArrowLeft size={14} />
                Back
              </Button>
            ) : null}
            <Button variant="ghost" size="md" onClick={close}>
              Cancel
            </Button>
            {!onForm ? null : isEdit ? (
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
      {onForm && current ? (
        <LoopForm
          draft={current}
          disabled={save.isPending || createBoardsLoading}
          autoFocusPrompt={!isEdit}
          onChange={setDraft}
        />
      ) : (
        <LoopTemplateGallery onPick={startFromTemplate} onBlank={startBlank} />
      )}
    </Modal>
  )
}
