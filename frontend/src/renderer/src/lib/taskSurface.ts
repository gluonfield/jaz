import type { ACPPermissionOption, PlanEntry, SessionEvent } from '@/lib/api/types'
import { isPlanApprovalPermission } from '@/lib/sessionPermissions'

export type TaskSurfaceKind = 'approval_plan' | 'progress'

export type PlanApprovalAction =
  | { type: 'message'; sessionId: string }
  | {
      type: 'permission'
      sessionId: string
      requestId: string
      approveOptionId: string
      clarifyOptionId?: string
    }

export interface TaskSurface {
  kind: TaskSurfaceKind
  title: string
  explanation?: string
  entries: PlanEntry[]
  awaitingApproval: boolean
  approval?: PlanApprovalAction
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

const approvePlanOptionPriority = ['bypassPermissions', 'auto', 'acceptEdits', 'default']

function preferredOption(options: ACPPermissionOption[] | undefined, ids: string[]) {
  for (const id of ids) {
    const option = options?.find((candidate) => candidate.id === id)
    if (option) return option
  }
  return undefined
}

function kindHasPrefix(option: ACPPermissionOption, prefix: string): boolean {
  return normalized(option.kind).startsWith(prefix)
}

function permissionApprovalAction(event: SessionEvent): PlanApprovalAction | undefined {
  const permission = event.permission
  if (!isPlanApprovalPermission(permission)) return undefined
  const options = permission.options ?? []
  const approveOption =
    preferredOption(options, approvePlanOptionPriority) ?? options.find((option) => kindHasPrefix(option, 'allow'))
  if (!approveOption) return undefined
  const clarifyOption =
    preferredOption(options, ['plan']) ?? options.find((option) => kindHasPrefix(option, 'reject'))
  return {
    type: 'permission',
    sessionId: event.session_id,
    requestId: permission.id,
    approveOptionId: approveOption.id,
    clarifyOptionId: clarifyOption?.id,
  }
}

export function approvalPlanSurfaceFromEvent(event: SessionEvent): TaskSurface | undefined {
  if (event.type === 'proposed_plan' && event.plan) {
    const awaitingApproval = Boolean(event.plan.awaiting_approval)
    const sessionId = event.acp?.id ?? event.session_id
    return {
      kind: 'approval_plan',
      title: 'Proposed Plan',
      explanation: event.plan.explanation || event.content,
      entries: event.plan.plan ?? [],
      awaitingApproval,
      approval: awaitingApproval ? { type: 'message', sessionId } : undefined,
      strikeCompleted: false,
    }
  }
  if (event.type === 'permission_request' && isPlanApprovalPermission(event.permission)) {
    const approval = permissionApprovalAction(event)
    return {
      kind: 'approval_plan',
      title: 'Proposed Plan',
      explanation: event.permission?.content,
      entries: [],
      awaitingApproval: Boolean(approval),
      approval,
      strikeCompleted: false,
    }
  }
  return undefined
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
    if (event.type === 'permission_request' && event.permission?.id) {
      return `approval_plan_permission:${event.permission.id}`
    }
    return `approval_plan:${event.acp?.id ?? event.session_id}`
  }
  return progressSignalKey(event)
}

export function taskSurfaceBelongsToSession(event: SessionEvent, sessionId: string): boolean {
  const acp = event.acp
  return acp ? acp.id === sessionId || acp.parent_id === sessionId : event.session_id === sessionId
}
