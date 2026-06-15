import type { ACPEvent, PlanEntry, SessionEvent } from '@/lib/api/types'

export type TaskSurfaceKind = 'approval_plan' | 'progress'

export interface TaskSurface {
  kind: TaskSurfaceKind
  title: string
  explanation?: string
  entries: PlanEntry[]
  awaitingApproval: boolean
  approvalSessionId?: string
  strikeCompleted: boolean
}

const activeStatuses = new Set(['in_progress', 'in-progress', 'running'])
const completedStatuses = new Set(['completed', 'complete'])
const maxProgressEntryContentLength = 240

function normalized(value?: string): string {
  return (value ?? '').trim().toLowerCase()
}

export type TaskStepState = 'pending' | 'active' | 'completed'

export function taskStepState(entry: PlanEntry): TaskStepState | undefined {
  const status = normalized(entry.status)
  if (!status) return undefined
  if (completedStatuses.has(status)) return 'completed'
  if (activeStatuses.has(status)) return 'active'
  return 'pending'
}

function hasActiveProgress(entries?: PlanEntry[]): boolean {
  return Boolean(entries?.some((entry) => taskStepState(entry) === 'active'))
}

function acpAwaitingApproval(acp: ACPEvent): boolean {
  const modes = acp.modes
  return Boolean(
    acp.plan?.length &&
      modes?.plan_mode_id &&
      modes.current_mode_id === modes.plan_mode_id &&
      normalized(acp.state) !== 'running' &&
      !hasActiveProgress(acp.plan),
  )
}

function progressEntries(entries?: PlanEntry[]): PlanEntry[] | undefined {
  if (!entries) return undefined
  const out: PlanEntry[] = []
  for (const entry of entries) {
    const content = progressEntryContent(entry.content)
    if (!content) return undefined
    out.push({ ...entry, content })
  }
  return out
}

function progressEntryContent(content?: string): string {
  const text = content?.trim() ?? ''
  if (!text || text.length > maxProgressEntryContentLength || /[\r\n]/.test(text)) return ''
  if (looksLikeMarkdownBlock(text)) return ''
  return text
}

function looksLikeMarkdownBlock(text: string): boolean {
  return (
    text.startsWith('# ') ||
    text.startsWith('##') ||
    text.startsWith('- ') ||
    text.startsWith('* ') ||
    text.startsWith('+ ') ||
    text.startsWith('> ') ||
    text.startsWith('```') ||
    text.startsWith('|') ||
    /^\d+[.)]\s/.test(text)
  )
}

export function approvalPlanSurfaceFromEvent(event: SessionEvent): TaskSurface | undefined {
  if (event.type === 'proposed_plan' && event.plan) {
    const awaitingApproval = Boolean(event.plan.awaiting_approval)
    return {
      kind: 'approval_plan',
      title: 'Proposed Plan',
      explanation: event.plan.explanation || event.content,
      entries: event.plan.plan ?? [],
      awaitingApproval,
      approvalSessionId: awaitingApproval ? event.session_id : undefined,
      strikeCompleted: false,
    }
  }
  const acp = event.acp
  if (acp?.plan?.length) {
    const entries = progressEntries(acp.plan)
    if (!entries) return undefined
    const awaitingApproval = acpAwaitingApproval({ ...acp, plan: entries })
    if (!awaitingApproval) return undefined
    return {
      kind: 'approval_plan',
      title: 'Proposed Plan',
      entries,
      awaitingApproval,
      approvalSessionId: acp.id,
      strikeCompleted: false,
    }
  }
  return undefined
}

export function progressSurfaceFromEvent(event: SessionEvent): TaskSurface | undefined {
  if (event.type === 'plan_update' && event.plan) {
    const entries = progressEntries(event.plan.plan)
    if (!entries) return undefined
    return {
      kind: 'progress',
      title: 'Progress',
      entries,
      awaitingApproval: false,
      strikeCompleted: true,
    }
  }
  const acp = event.acp
  if (acp?.plan?.length) {
    const entries = progressEntries(acp.plan)
    if (!entries) return undefined
    const awaitingApproval = acpAwaitingApproval({ ...acp, plan: entries })
    if (awaitingApproval) return undefined
    return {
      kind: 'progress',
      title: 'Progress',
      entries,
      awaitingApproval: false,
      strikeCompleted: false,
    }
  }
  return undefined
}

export function taskSurfaceFromEvent(event: SessionEvent): TaskSurface | undefined {
  return approvalPlanSurfaceFromEvent(event) ?? progressSurfaceFromEvent(event)
}

export function taskSurfaceKey(event: SessionEvent): string {
  if (approvalPlanSurfaceFromEvent(event)) {
    return `approval_plan:${event.acp?.id ?? event.session_id}`
  }
  if (event.type === 'plan_update' && event.plan && progressSurfaceFromEvent(event)) {
    return `progress:${event.session_id}`
  }
  if (event.acp?.plan?.length && progressSurfaceFromEvent(event)) return `progress:${event.acp.id}`
  return ''
}

export function taskSurfaceBelongsToSession(event: SessionEvent, sessionId: string): boolean {
  const acp = event.acp
  return acp ? acp.id === sessionId || acp.parent_id === sessionId : event.session_id === sessionId
}
