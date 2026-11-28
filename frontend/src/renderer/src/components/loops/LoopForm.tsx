import { useQuery } from '@tanstack/react-query'
import { type ReactNode, useEffect, useState } from 'react'
import { MentionSuggestions, MentionTextarea, useMentionInput } from '@/components/session/MentionInput'
import { ModelSelect } from '@/components/session/ModelSelect'
import { boardsQuery } from '@/lib/api/boards'
import { useAgentModelCatalog } from '@/lib/hooks/useAgentModelCatalog'
import { pickerEffortOptions } from '@/lib/modelPicker'
import { BoardAssignmentPicker } from './BoardAssignmentPicker'
import { LoopExamplesPicker } from './LoopExamplesPicker'
import type { LoopDraft } from './loopDraft'
import { templatePatch } from './loopTemplates'
import { SchedulePicker } from './SchedulePicker'

// A patch updater shared by every step: merges a partial draft into the whole.
type SetDraft = (patch: Partial<LoopDraft>) => void

// Borderless like the composer: the surface fill is the field. A primary ring
// appears only on focus, never as a resting outline.
const inputClass =
  'w-full rounded-control bg-surface px-3 py-2 text-[13px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:ring-1 focus:ring-primary'

function Field({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
    </label>
  )
}

// The explanatory line above each step's input, with an optional right-aligned
// action (the Prompt step hangs its Examples button here).
function StepHeader({ description, action }: { description: string; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <p className="text-pretty text-[13px] text-ink-2">{description}</p>
      {action}
    </div>
  )
}

// Step 1 — the instruction: a mention-capable prompt, examples to seed it, and
// an optional name.
export function PromptStep({
  draft,
  disabled,
  autoFocus,
  description,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  autoFocus?: boolean
  description: string
  set: SetDraft
}) {
  // The prompt seeds its text at mount, so applying an example must remount the
  // card — bumping `seed` does exactly that.
  const [seed, setSeed] = useState(0)
  const [examplesOpen, setExamplesOpen] = useState(false)

  return (
    <div className="space-y-3">
      <StepHeader
        description={description}
        action={
          <button
            type="button"
            disabled={disabled}
            onClick={() => setExamplesOpen(true)}
            className="shrink-0 text-[12px] font-medium text-primary transition-colors hover:text-primary-strong disabled:opacity-50"
          >
            Examples
          </button>
        }
      />
      <LoopPromptCard key={seed} draft={draft} disabled={disabled} autoFocus={autoFocus} set={set} />
      <LoopExamplesPicker
        open={examplesOpen}
        onClose={() => setExamplesOpen(false)}
        onPick={(template) => {
          set(templatePatch(template))
          setSeed((s) => s + 1)
          setExamplesOpen(false)
        }}
      />

      <Field label={<>Name <span className="font-normal text-ink-3">optional</span></>}>
        <input
          type="text"
          disabled={disabled}
          value={draft.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder="daily-code-review"
          className={inputClass}
        />
      </Field>
    </div>
  )
}

// Step 2 — when it runs.
export function ScheduleStep({
  draft,
  disabled,
  description,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  description: string
  set: SetDraft
}) {
  return (
    <div className="space-y-4">
      <StepHeader description={description} />
      <SchedulePicker
        value={draft.schedule}
        disabled={disabled}
        onChange={(schedule) => set({ schedule })}
      />
    </div>
  )
}

// Step 3 — which boards carry the loop's live app.
export function BoardsStep({
  draft,
  disabled,
  description,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  description: string
  set: SetDraft
}) {
  const boards = useQuery(boardsQuery)

  const body =
    boards.isPending ? (
      <span className="text-[12px] text-ink-3">Loading boards…</span>
    ) : boards.isError || (boards.data ?? []).length === 0 ? (
      <p className="text-[12px] text-ink-3">
        No boards yet. Create one in Apps to give this loop a live app.
      </p>
    ) : (
      <BoardAssignmentPicker
        boards={boards.data}
        selected={draft.boardIds}
        disabled={disabled}
        onChange={(boardIds) => set({ boardIds })}
      />
    )

  return (
    <div className="space-y-4">
      <StepHeader description={description} />
      {body}
    </div>
  )
}

// The composer-style prompt card: a mention-capable textarea ($skill / @file)
// with the run setup (agent, model) as its toolbar. The draft keeps `directory`
// — it scopes @-file mentions — even though there's no project picker; keep it.
function LoopPromptCard({
  draft,
  disabled,
  autoFocus,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  autoFocus?: boolean
  set: SetDraft
}) {
  const mention = useMentionInput({
    fileRoot: draft.directory,
    disabled,
    maxHeight: 240,
    initialValue: draft.prompt,
    onValueChange: (prompt) => set({ prompt }),
  })

  const catalog = useAgentModelCatalog({
    agent: draft.runtime,
    provider: draft.provider,
    model: draft.model,
    effort: draft.reasoningEffort,
  })
  const runtimeAvailable = catalog.ready && catalog.agents.length > 0

  useEffect(() => {
    if (!catalog.ready || catalog.defaultAgent === draft.runtime) return
    if (!catalog.defaultAgent) {
      set({ runtime: '', provider: '', model: '', reasoningEffort: '' })
      return
    }
    const next = catalog.pickAgent(catalog.defaultAgent)
    set({
      runtime: next.agent,
      provider: next.provider,
      model: catalog.usingCloud ? catalog.model : next.model,
      reasoningEffort: catalog.usingCloud ? catalog.effort : next.effort,
    })
  }, [catalog, draft.runtime, set])

  const reasoningEffortOverride = catalog.usingCloud
    ? catalog.effort
    : catalog.effort === catalog.requestedEffort
      ? draft.reasoningEffort
      : catalog.effort
  useEffect(() => {
    if (!catalog.ready || (catalog.usingCloud && !catalog.agent)) return
    if (draft.reasoningEffort !== reasoningEffortOverride) {
      set({ reasoningEffort: reasoningEffortOverride })
    }
  }, [catalog.agent, catalog.ready, catalog.usingCloud, draft.reasoningEffort, reasoningEffortOverride, set])

  return (
    <div>
      <div className="relative">
        <MentionSuggestions mention={mention} placement="above" />
        <div
          className="flex cursor-text flex-col gap-2 rounded-card bg-surface p-2.5"
          onClick={(e) => {
            if ((e.target as HTMLElement).closest('button, textarea, input')) return
            mention.textareaRef.current?.focus()
          }}
        >
          <MentionTextarea
            mention={mention}
            placeholder="Review yesterday's commits and flag anything concerning…"
            disabled={disabled}
            autoFocus={autoFocus}
            minHeightClass="min-h-[54px]"
          />
          <div className="flex flex-wrap items-center gap-1.5">
            {catalog.ready && !runtimeAvailable ? (
              <span className="px-1.5 text-[13px] text-ink-3">
                {catalog.usingCloud ? 'No cloud models configured' : 'Connect an agent in Settings'}
              </span>
            ) : null}
            {runtimeAvailable ? (
              <ModelSelect
                agent={{
                  value: catalog.agent || draft.runtime,
                  options: catalog.agents,
                  items: catalog.agentItems,
                  onChange: (runtime) => {
                    const next = catalog.pickAgent(runtime)
                    set({ runtime: next.agent, provider: next.provider, model: next.model, reasoningEffort: next.effort })
                  },
                }}
                value={catalog.model}
                suggestions={catalog.modelSuggestions}
                loading={catalog.loading}
                disabled={disabled}
                placement="below"
                onChange={(next) => set({ model: next.model, reasoningEffort: next.effort })}
                effort={catalog.effort}
                effortOptions={pickerEffortOptions(catalog.effortOptions)}
              />
            ) : null}
          </div>
        </div>
      </div>
      <span className="mt-1.5 block text-[12px] text-ink-3">
        Type $ to tag a skill, @ to tag a file or thread.
      </span>
    </div>
  )
}
