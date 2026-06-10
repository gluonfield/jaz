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

function normalized(value?: string): string {
  return (value ?? '').trim().toLowerCase()
}

function hasActiveProgress(entries?: PlanEntry[]): boolean {
  return Boolean(entries?.some((entry) => activeStatuses.has(normalized(entry.status))))
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
