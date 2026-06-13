import type { ACPEvent, PlanEntry, SessionEvent } from '@/lib/api/types'

export interface PlanSurface {
  title: string
  explanation?: string
  entries: PlanEntry[]
  awaitingApproval: boolean
  approvalSessionId?: string
  strikeCompleted: boolean
}

const activeStatuses = new Set(['in_progress', 'in-progress', 'running'])
const completedStatuses = new Set(['completed', 'complete'])

function normalized(value?: string): string {
  return (value ?? '').trim().toLowerCase()
}

export type PlanStepState = 'pending' | 'active' | 'completed'

// undefined means the entry carries no status, so the UI should not render a
// fake checkbox for it.
export function planStepState(entry: PlanEntry): PlanStepState | undefined {
  const status = normalized(entry.status)
  if (!status) return undefined
  if (completedStatuses.has(status)) return 'completed'
  if (activeStatuses.has(status)) return 'active'
  return 'pending'
}

function hasActiveProgress(entries?: PlanEntry[]): boolean {
  return Boolean(entries?.some((entry) => planStepState(entry) === 'active'))
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

export function planSurfaceFromEvent(event: SessionEvent): PlanSurface | undefined {
  if (event.type === 'plan_update' && event.plan) {
    return {
      title: 'Updated Plan',
      explanation: event.plan.explanation,
      entries: event.plan.plan ?? [],
      awaitingApproval: false,
      strikeCompleted: true,
    }
  }
  if (event.type === 'proposed_plan' && event.plan) {
    const awaitingApproval = Boolean(event.plan.awaiting_approval)
    return {
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
    const awaitingApproval = acpAwaitingApproval(acp)
    return {
      title: 'Plan',
      entries: acp.plan,
      awaitingApproval,
      approvalSessionId: awaitingApproval ? acp.id : undefined,
      strikeCompleted: false,
    }
  }
  return undefined
}

export function planProgressSurfaceFromEvent(event: SessionEvent): PlanSurface | undefined {
  if (event.type === 'proposed_plan') return undefined
  const surface = planSurfaceFromEvent(event)
  return surface && !surface.awaitingApproval ? surface : undefined
}

export function planSurfaceKey(event: SessionEvent): string {
  if (event.type === 'plan_update' && event.plan) return `plan_update:${event.session_id}`
  if (event.type === 'proposed_plan' && event.plan) return `proposed_plan:${event.session_id}`
  if (event.acp?.plan?.length) return `acp_plan:${event.acp.id}`
  return ''
}

export function planSurfaceBelongsToSession(event: SessionEvent, sessionId: string): boolean {
  const acp = event.acp
  return acp ? acp.id === sessionId || acp.parent_id === sessionId : event.session_id === sessionId
}
