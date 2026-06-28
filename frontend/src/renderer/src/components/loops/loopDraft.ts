import type { LoopInput } from '@/lib/api/loops'
import type { AgentSettings, Loop } from '@/lib/api/types'
import { acpUsesModelProvider } from '@/lib/agentRuntimes'
import {
  type ScheduleDraft,
  cronFromDraft,
  defaultScheduleDraft,
  draftFromLoop,
  localTimezone,
} from './schedule'

// The editable shape behind the create/edit modal. `runtime` is an ACP agent
// name, matching the RuntimeSelect contract used by the new-thread composer.
export interface LoopDraft {
  name: string
  prompt: string
  runtime: string
  directory: string
  // Overrides of the Settings > Agents defaults; '' follows settings at run
  // time, while the picker always displays the resolved effective value.
  provider: string
  model: string
  reasoningEffort: string
  schedule: ScheduleDraft
  // Boards the loop's widget lives on; assignment is the widget enablement.
  boardIds: string[]
}

export function emptyLoopDraft(boardIds: string[] = []): LoopDraft {
  return {
    name: '',
    prompt: '',
    runtime: '',
    directory: '',
    provider: '',
    model: '',
    reasoningEffort: '',
    schedule: defaultScheduleDraft(),
    boardIds,
  }
}

export function loopDraftFromLoop(loop: Loop, boardIds: string[] = []): LoopDraft {
  return {
    name: loop.name ?? '',
    prompt: loop.prompt ?? '',
    runtime: loop.acp_agent || '',
    directory: loop.directory ?? '',
    provider: loop.model_provider ?? '',
    model: loop.model ?? '',
    reasoningEffort: loop.reasoning_effort ?? '',
    schedule: draftFromLoop(loop.schedule?.expr ?? '', loop.status === 'paused'),
    boardIds,
  }
}

// Whether the given step (Prompt=0, Schedule=1, Boards=2) is complete enough to
// advance. The final submit still goes through canSaveLoop, which is the union
// of every step's requirement.
export function stepValid(draft: LoopDraft, step: number): boolean {
  if (step === 0) return draft.prompt.trim() !== '' && draft.runtime.trim() !== ''
  if (step === 1) return !(draft.schedule.preset === 'custom' && draft.schedule.expr.trim() === '')
  return true
}

export function canSaveLoop(draft: LoopDraft): boolean {
  return stepValid(draft, 0) && stepValid(draft, 1) && stepValid(draft, 2)
}

export function loopDraftToInput(draft: LoopDraft, settings?: AgentSettings): LoopInput {
  const usesModelProvider = acpUsesModelProvider(settings, draft.runtime)
  return {
    prompt: draft.prompt.trim(),
    name: draft.name.trim() || undefined,
    schedule: { kind: 'cron', expr: cronFromDraft(draft.schedule), timezone: localTimezone() },
    status: draft.schedule.preset === 'manual' ? 'paused' : 'active',
    runtime: 'acp',
    acp_agent: draft.runtime,
    // Overrides are always sent: '' clears one back to following settings.
    model_provider: usesModelProvider ? draft.provider : '',
    model: draft.model,
    reasoning_effort: draft.reasoningEffort,
    directory: draft.directory || undefined,
    // Always sent: an empty list unassigns the widget from every board.
    board_ids: draft.boardIds,
  }
}
