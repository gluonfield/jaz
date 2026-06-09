import type { ReactNode } from 'react'
import { DirectoryPicker, RuntimeSelect } from '@/components/session/NewThreadControls'
import type { Loop } from '@/lib/api/types'
import type { LoopInput } from '@/lib/api/loops'
import { ReasoningEffortSelect } from './ReasoningEffortSelect'
import { SchedulePicker } from './SchedulePicker'
import {
  type ScheduleDraft,
  cronFromDraft,
  defaultScheduleDraft,
  draftFromLoop,
  localTimezone,
} from './schedule'

// 'native' selects the native runtime; any other value is the ACP agent name —
// matching the RuntimeSelect contract used by the new-thread composer.
export interface LoopDraft {
  name: string
  prompt: string
  runtime: string
  directory: string
  reasoningEffort: string
  schedule: ScheduleDraft
}

export function emptyLoopDraft(agents: string[]): LoopDraft {
  return {
    name: '',
    prompt: '',
    runtime: agents.includes('codex') ? 'codex' : (agents[0] ?? 'native'),
    directory: '',
    reasoningEffort: '',
    schedule: defaultScheduleDraft(),
  }
}

export function loopDraftFromLoop(loop: Loop): LoopDraft {
  return {
    name: loop.name ?? '',
    prompt: loop.prompt ?? '',
    runtime: loop.runtime === 'acp' ? (loop.acp_agent || 'codex') : 'native',
    directory: loop.directory ?? '',
    reasoningEffort: loop.reasoning_effort ?? '',
    schedule: draftFromLoop(loop.schedule?.expr ?? '', loop.status === 'paused'),
  }
}

export function canSaveLoop(draft: LoopDraft): boolean {
  if (draft.prompt.trim() === '') return false
  if (draft.schedule.preset === 'custom' && draft.schedule.expr.trim() === '') return false
  return true
}

export function loopDraftToInput(draft: LoopDraft): LoopInput {
  const native = draft.runtime === 'native'
  return {
    prompt: draft.prompt.trim(),
    name: draft.name.trim() || undefined,
    schedule: { kind: 'cron', expr: cronFromDraft(draft.schedule), timezone: localTimezone() },
    status: draft.schedule.preset === 'manual' ? 'paused' : 'active',
    runtime: native ? 'native' : 'acp',
    acp_agent: native ? undefined : draft.runtime,
    reasoning_effort: draft.reasoningEffort || undefined,
    directory: native ? undefined : draft.directory || undefined,
  }
}

const inputClass =
  'w-full rounded-control border border-border bg-bg px-3 py-2 text-[13px] text-ink outline-none transition-colors duration-150 placeholder:text-ink-3 focus:border-primary focus:ring-2 focus:ring-primary/15'

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
      {hint ? <span className="mt-1 block text-[12px] text-ink-3">{hint}</span> : null}
    </label>
  )
}

// Like Field but a plain block — used for groups of controls (the agent pills,
// the schedule picker) where a <label> would forward hover/click to one child.
function FieldGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
    </div>
  )
}

export function LoopForm({
  draft,
  agents,
  disabled,
  onChange,
}: {
  draft: LoopDraft
  agents: string[]
  disabled?: boolean
  onChange: (next: LoopDraft) => void
}) {
  const set = (patch: Partial<LoopDraft>) => onChange({ ...draft, ...patch })

  return (
    <div className="space-y-5">
      <Field label="Name" hint="Optional — defaults to the start of the prompt.">
        <input
          type="text"
          disabled={disabled}
          value={draft.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder="daily-code-review"
          className={inputClass}
        />
      </Field>

      <Field label="Prompt" hint="Sent to a fresh thread on each run.">
        <textarea
          rows={4}
          disabled={disabled}
          value={draft.prompt}
          onChange={(e) => set({ prompt: e.target.value })}
          placeholder="Review yesterday's commits and flag anything concerning…"
          className={`${inputClass} resize-y`}
        />
      </Field>

      <FieldGroup label="Agent">
        <div className="flex flex-wrap items-center gap-2">
          <RuntimeSelect
            value={draft.runtime}
            agents={agents}
            disabled={disabled}
            onChange={(runtime) => set({ runtime })}
          />
          {draft.runtime !== 'native' ? (
            <DirectoryPicker
              value={draft.directory}
              disabled={disabled}
              onChange={(directory) => set({ directory })}
            />
          ) : null}
          <ReasoningEffortSelect
            value={draft.reasoningEffort}
            disabled={disabled}
            onChange={(reasoningEffort) => set({ reasoningEffort })}
          />
        </div>
      </FieldGroup>

      <FieldGroup label="Schedule">
        <SchedulePicker
          value={draft.schedule}
          disabled={disabled}
          onChange={(schedule) => set({ schedule })}
        />
      </FieldGroup>
    </div>
  )
}
