import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { Fragment, useState } from 'react'
import { BoardsStep, PromptStep, ScheduleStep } from '@/components/loops/LoopForm'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { boardsQuery } from '@/lib/api/boards'
import { createLoop, runLoopNow, updateLoop } from '@/lib/api/loops'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { Loop } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import {
  type LoopDraft,
  canSaveLoop,
  emptyLoopDraft,
  loopDraftFromLoop,
  loopDraftToInput,
  stepValid,
} from './loopDraft'

// Step presentation in order; indices line up with stepValid in loopDraft.
const STEPS = [
  { label: 'Prompt', description: 'What should this loop do on each run?' },
  { label: 'Schedule', description: 'How often should it run?' },
  { label: 'Boards', description: 'Show its latest run as a live widget on your boards.' },
]
const LAST_STEP = STEPS.length - 1

// One modal for both create (no `loop`) and edit (`loop` provided), walked as a
// Prompt → Schedule → Boards stepper. Edit never happens inline on the detail
// page — it always opens here.
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
  const [step, setStep] = useState(0)
  const createBoardIds = boards.data?.map((board) => board.id) ?? []
  const createBoardsLoading = !isEdit && boards.isPending
  const current = draft ?? (loop ? loopDraftFromLoop(loop, boardIds) : emptyLoopDraft(createBoardIds))
  const set = (patch: Partial<LoopDraft>) => setDraft({ ...current, ...patch })

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
    setStep(0)
    save.reset()
    onClose()
  }

  const onLastStep = step === LAST_STEP
  const canSubmit = canSaveLoop(current) && !save.isPending && !createBoardsLoading
  const reduce = useReducedMotion()

  return (
    <Modal
      open={open}
      onClose={close}
      size="md"
      title={isEdit ? 'Edit loop' : 'New loop'}
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {save.isError ? save.error.message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            {step > 0 ? (
              <Button variant="ghost" size="md" onClick={() => setStep(step - 1)}>
                Back
              </Button>
            ) : (
              <Button variant="ghost" size="md" onClick={close}>
                Cancel
              </Button>
            )}
            {!onLastStep ? (
              <Button
                variant="primary"
                size="md"
                disabled={!stepValid(current, step)}
                onClick={() => setStep(step + 1)}
              >
                Next
              </Button>
            ) : isEdit ? (
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
      <StepNav step={step} onJump={setStep} />
      <AnimatePresence mode="wait" initial={false}>
        <motion.div
          key={step}
          initial={reduce ? false : { opacity: 0, x: 6 }}
          animate={{ opacity: 1, x: 0 }}
          exit={reduce ? { opacity: 0 } : { opacity: 0, x: -6 }}
          transition={{ duration: 0.16, ease: [0.2, 0, 0, 1] }}
          className="mt-5 space-y-3"
        >
          <p className="text-pretty text-[13px] text-ink-2">{STEPS[step].description}</p>
          {step === 0 ? (
            <PromptStep draft={current} disabled={save.isPending} autoFocus set={set} />
          ) : step === 1 ? (
            <ScheduleStep draft={current} disabled={save.isPending} set={set} />
          ) : (
            <BoardsStep draft={current} disabled={save.isPending} set={set} />
          )}
        </motion.div>
      </AnimatePresence>
    </Modal>
  )
}

// Minimal progress trail. Completed steps are clickable to go back; later steps
// are reached with Next so their entry requirements stay enforced.
function StepNav({ step, onJump }: { step: number; onJump: (step: number) => void }) {
  return (
    <div className="flex items-center gap-1.5 text-[12px]">
      {STEPS.map(({ label }, i) => (
        <Fragment key={label}>
          {i > 0 ? <ChevronRight size={12} className="text-ink-3/60" /> : null}
          <button
            type="button"
            disabled={i > step}
            onClick={() => onJump(i)}
            className={
              i === step
                ? 'font-medium text-ink'
                : i < step
                  ? 'text-ink-3 transition-colors hover:text-ink'
                  : 'text-ink-3'
            }
          >
            {label}
          </button>
        </Fragment>
      ))}
    </div>
  )
}
