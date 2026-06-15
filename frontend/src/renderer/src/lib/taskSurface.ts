import type { PlanEntry, SessionEvent } from '@/lib/api/types'

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
  if (!text || Array.from(text).length > maxProgressEntryContentLength || /[\r\n]/.test(text)) {
    return ''
  }
  if (looksLikeMarkdownBlock(text)) return ''
  return text
}

function hasACPPlanSignal(acp: SessionEvent['acp']): boolean {
  return Boolean(acp && Object.prototype.hasOwnProperty.call(acp, 'plan'))
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
  if (event.type !== 'proposed_plan' || !event.plan) return undefined
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

export function progressSurfaceFromEvent(event: SessionEvent): TaskSurface | undefined {
  if (event.type === 'plan_update' && event.plan) {
    const entries = progressEntries(event.plan.plan)
    if (!entries?.length) return undefined
    return {
      kind: 'progress',
      title: 'Progress',
      entries,
      awaitingApproval: false,
      strikeCompleted: true,
    }
  }
  const acp = event.acp
  if (acp && hasACPPlanSignal(acp)) {
    const entries = progressEntries(acp.plan)
    if (!entries?.length) return undefined
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

export function progressSignalKey(event: SessionEvent): string {
  if (event.type === 'plan_update' && event.plan) return `progress:${event.session_id}`
  const acp = event.acp
  if (acp && hasACPPlanSignal(acp) && acp.id) return `progress:${acp.id}`
  return ''
}

export function hasProgressSignal(event: SessionEvent): boolean {
  return Boolean(progressSignalKey(event))
}

export function taskSurfaceKey(event: SessionEvent): string {
  if (approvalPlanSurfaceFromEvent(event)) {
    return `approval_plan:${event.acp?.id ?? event.session_id}`
  }
  return progressSignalKey(event)
}

export function taskSurfaceBelongsToSession(event: SessionEvent, sessionId: string): boolean {
  const acp = event.acp
  return acp ? acp.id === sessionId || acp.parent_id === sessionId : event.session_id === sessionId
}
