import type {
  ACPJobSnapshot,
  ACPModeState,
  ChatMessage,
  GoalEvent,
  Session,
  SessionEvent,
  SessionMessages,
} from '@/lib/api/types'
import { runtimeCapabilitiesSupportNativeGoal } from '@/lib/agentRuntimes'
import { applyProviderToolTitleFallbacks, providerSubagentsFromEvents } from '@/lib/providerSubagents'
import { spawnedThreadsFromSources } from '@/lib/spawnedThreads'
import {
  approvalPlanSurfaceFromEvent,
  hasProgressSignal,
  progressSurfaceFromEvent,
  taskSurfaceBelongsToSession,
} from '@/lib/taskSurface'
import { coalesceSessionEvents, sessionEventPlacement } from '@/lib/sessionEvents'
import {
  activePermissionIDs,
  isPermissionAwaitingResponse,
  planApprovalPermissionIDs,
  resolveInactivePermissions,
} from '@/lib/sessionPermissions'

export function isCodexACPSession(session: Session | undefined): boolean {
  return session?.runtime === 'acp' && session.runtime_ref?.agent?.trim().toLowerCase() === 'codex'
}

export function sessionSupportsNativeGoal(session: Session | undefined): boolean {
  return session?.runtime === 'acp' && runtimeCapabilitiesSupportNativeGoal(session.runtime_ref?.capabilities)
}

export function latestGoalEvent(sessionId: string, events: SessionEvent[]): GoalEvent | null | undefined {
  const event = events.findLast((item) => (
    item.session_id === sessionId &&
    (item.type === 'goal_clear' || (item.type === 'goal_update' && item.goal))
  ))
  if (!event) return undefined
  return event.type === 'goal_clear' ? null : event.goal
}

export function goalIsActive(goal: GoalEvent | undefined): boolean {
  return Boolean(goal?.status && goal.status !== 'complete')
}

export function latestACPGoalRequested(sessionId: string, events: SessionEvent[]): boolean | undefined {
  let latest: boolean | undefined
  for (const event of events) {
    if (event.acp?.id !== sessionId || event.acp.goal_requested === undefined) continue
    latest = event.acp.goal_requested
  }
  return latest
}

export function stripACPError(event: SessionEvent): SessionEvent {
  if (!event.acp?.error) return event
  return { ...event, acp: { ...event.acp, error: undefined } }
}

export function stripProgressSignal(event: SessionEvent): SessionEvent {
  if (event.acp) {
    const { plan: _plan, ...acp } = event.acp
    return { ...event, acp }
  }
  const { plan: _plan, ...out } = event
  return out
}

export function sessionEventErrorMessage(event: SessionEvent): string {
  return event.acp?.error?.trim() ?? ''
}

export function modeStateKnown(modes?: ACPModeState): boolean {
  return Boolean(
    modes?.plan_mode_id ||
      modes?.current_mode_id ||
      modes?.available_modes?.length,
  )
}

export function planModeActive(modes?: ACPModeState): boolean {
  return Boolean(modes?.plan_mode_id && modes.current_mode_id === modes.plan_mode_id)
}

export function latestACPModeState(sessionId: string, events: SessionEvent[]): ACPModeState | undefined {
  let latest: ACPModeState | undefined
  for (const event of events) {
    if (event.acp?.id !== sessionId || !modeStateKnown(event.acp.modes)) continue
    latest = event.acp.modes
  }
  return latest
}

function acpSnapshotEvents(job: ACPJobSnapshot): SessionEvent[] {
  const at = job.last_event_at || job.updated_at
  const events: SessionEvent[] = []
  if (
    job.assistant ||
    job.thought ||
    job.plan?.length ||
    job.tool_calls?.length ||
    job.error
  ) {
    events.push({
      session_id: job.id,
      type: 'acp',
      at,
      content: job.assistant,
      acp: {
        id: job.id,
        slug: job.slug,
        title: job.title,
        parent_id: job.parent_id,
        agent: job.acp_agent,
        session_id: job.acp_session,
        model_provider: job.model_provider,
        model: job.model,
        reasoning_effort: job.reasoning_effort,
        state: job.state,
        stop_reason: job.stop_reason,
        assistant: job.assistant,
        thought: job.thought,
        error: job.error,
        modes: job.modes,
        plan: job.plan,
        tool_calls: job.tool_calls,
        permissions: job.permissions,
        goal_requested: job.goal_requested,
        last_event_at: job.last_event_at,
        last_tool_at: job.last_tool_at,
      },
    })
  }
  if (isACPStateRunning(job.state)) {
    for (const permission of job.permissions ?? []) {
      if (!isPermissionAwaitingResponse(permission)) continue
      events.push({
        session_id: job.id,
        type: 'permission_request',
        at,
        permission,
      })
    }
  }
  return events
}

function isACPStateRunning(state?: string): boolean {
  return state === 'running' || state === 'starting'
}

function sanitizeParentChildACPEvent(event: SessionEvent, pageSessionID: string): SessionEvent | null {
  const acp = event.acp
  if (!acp || acp.parent_id !== pageSessionID || acp.id === pageSessionID) return event
  if (event.type === 'acp_message' || event.type === 'acp_thought' || event.type === 'acp_tool') {
    return null
  }
  return {
    ...event,
    content: undefined,
    acp: {
      ...acp,
      assistant: undefined,
      thought: undefined,
      tool_calls: undefined,
      permissions: undefined,
    },
  }
}

function hasStoredAssistantMessage(messages: ChatMessage[], text?: string): boolean {
  const expected = text?.trim()
  if (!expected) return false
  return messages.some((message) => {
    if (message.role !== 'assistant') return false
    const blockText = message.blocks
      ?.filter((block) => block.type === 'text')
      .map((block) => (block.text ?? '').trim())
      .filter(Boolean)
      .join('\n\n')
    return message.content.trim() === expected || blockText === expected
  })
}

// Everything the page derives from server data, computed once per data change
// (one useMemo) so renders driven by unrelated state — panel width ticks,
// composer state, live stream flags — skip the O(events) pipeline.
export type SessionView = ReturnType<typeof deriveSessionView>

export function deriveSessionView(data: SessionMessages, liveEvents: SessionEvent[]) {
  const {
    session,
    messages,
    acp_state: acpState,
    acp_assistant: acpAssistant,
    acp_thought: acpThought,
    acp_modes: acpModes,
    acp_plan: acpPlan,
    acp_tool_calls: acpToolCalls,
    acp_permissions: acpPermissions,
    acp_error: acpError,
    acp_goal_requested: acpGoalRequested,
    acp_active_operation: acpActiveOperation,
    acp_last_event_at: acpLastEventAt,
    acp_last_tool_at: acpLastToolAt,
    acp_children: acpChildren,
    acp_child_permissions: acpChildPermissions,
    events: persistedEvents = [],
  } = data
  // The job snapshot is only a fallback: when events record the run, the
  // snapshot would repeat it and dump every tool call at the end.
  const eventsCoverOwnACP =
    [...persistedEvents, ...liveEvents].some(
      (event) =>
        (event.type === 'acp_message' || event.type === 'acp_thought' || event.type === 'acp_tool') &&
        event.acp?.id === session.id,
    ) || hasStoredAssistantMessage(messages, acpAssistant)
  const snapshotEvents: SessionEvent[] = [
    ...(session.runtime === 'acp'
      ? acpSnapshotEvents(
          {
            id: session.id,
            slug: session.slug,
            title: session.title,
            parent_id: session.parent_id,
            acp_agent: session.runtime_ref?.agent ?? 'acp',
            acp_session: session.runtime_ref?.session_id ?? '',
            model_provider: session.model_provider,
            model: session.model,
            reasoning_effort: session.reasoning_effort,
            state: acpState ?? session.status,
            assistant: eventsCoverOwnACP ? undefined : acpAssistant,
            thought: eventsCoverOwnACP ? undefined : acpThought,
            error: acpError,
            modes: acpModes,
            plan: acpPlan,
            tool_calls: eventsCoverOwnACP ? undefined : acpToolCalls,
            permissions: acpPermissions,
            active_operation: acpActiveOperation,
            last_event_at: acpLastEventAt,
            last_tool_at: acpLastToolAt,
            updated_at: session.updated_at,
          })
      : []),
  ]
  const modeEvents = [...persistedEvents, ...snapshotEvents, ...liveEvents]
  const currentModes = latestACPModeState(session.id, modeEvents) ?? acpModes
  const livePermissionEvents = [...snapshotEvents, ...liveEvents]
  const activePermissions = activePermissionIDs(livePermissionEvents, acpChildPermissions)
  const activePlanApprovalPermissions = planApprovalPermissionIDs(livePermissionEvents, acpChildPermissions)
  const rawTranscriptEvents = applyProviderToolTitleFallbacks(
    [...persistedEvents, ...snapshotEvents, ...liveEvents].flatMap((event) => {
      // 'assistant' events are refresh signals; the message store has the content.
      if (event.type === 'assistant' || sessionEventPlacement(event) === 'side_chat') return []
      // Old rows round-tripped a typed-nil ACP into an empty struct.
      if (event.acp && !event.acp.id) event = { ...event, acp: undefined }
      const sanitized = sanitizeParentChildACPEvent(event, session.id)
      return sanitized ? [sanitized] : []
    }),
  )
  const transcriptEvents = coalesceSessionEvents(rawTranscriptEvents)
  const settledTranscriptEvents = resolveInactivePermissions(transcriptEvents, activePermissions)
  const acpModesKnown = modeStateKnown(currentModes)
  const planAvailable = session.runtime !== 'acp' || !acpModesKnown || Boolean(currentModes?.plan_mode_id)
  const planActive = planModeActive(currentModes)
  const goalAvailable = sessionSupportsNativeGoal(session)
  const runtimeEvents = [...persistedEvents, ...snapshotEvents, ...liveEvents]
  const latestGoal = latestGoalEvent(session.id, runtimeEvents)
  const goal = latestGoal === null ? undefined : latestGoal ?? session.goal
  const goalTurnRequested = latestACPGoalRequested(session.id, runtimeEvents) ?? Boolean(acpGoalRequested)
  const goalActive = goalIsActive(goal) || goalTurnRequested
  const hasBlockingPendingPermission = Array.from(activePermissions).some(
    (id) => !activePlanApprovalPermissions.has(id),
  )
  const latestUserAt = Math.max(
    0,
    ...messages
      .filter((message) => message.role === 'user')
      .map((message) => Date.parse(message.created_at))
      .filter((time) => !Number.isNaN(time)),
  )
  const latestPlanDecisionEvent = settledTranscriptEvents.findLast((event) => {
    const surface = approvalPlanSurfaceFromEvent(event)
    return Boolean(
      surface?.awaitingApproval &&
        surface.approval &&
        (surface.approval.type !== 'permission' ||
          activePlanApprovalPermissions.has(surface.approval.requestId)) &&
        taskSurfaceBelongsToSession(event, session.id),
    )
  })
  const latestPlanDecisionSurface = latestPlanDecisionEvent
    ? approvalPlanSurfaceFromEvent(latestPlanDecisionEvent)
    : undefined
  const planDecisionAt = Date.parse(latestPlanDecisionEvent?.at ?? '')
  const panelProgressEvent = settledTranscriptEvents.findLast((event) =>
    Boolean(progressSurfaceFromEvent(event) && taskSurfaceBelongsToSession(event, session.id)),
  )
  // Progress lives in the side panel, never in the thread; only a proposed
  // plan that needs the user's approval stays inline. Errors are
  // notified as toasts, not rendered as rows.
  const displayEvents = coalesceSessionEvents(
    settledTranscriptEvents.flatMap((event) => {
      if (sessionEventPlacement(event) !== 'transcript') return []
      const withoutError = stripACPError(event)
      if (!hasProgressSignal(withoutError)) return [withoutError]
      return [stripProgressSignal(withoutError)]
    }),
  )
  const sideChatEvents = coalesceSessionEvents(
    [...persistedEvents, ...liveEvents].filter((event) => sessionEventPlacement(event) === 'side_chat'),
  )
  return {
    transcriptEvents: settledTranscriptEvents,
    sideChatEvents,
    displayEvents,
    latestUserAt,
    planAvailable,
    planActive,
    goalAvailable,
    goalActive,
    goalTurnRequested,
    goal,
    hasBlockingPendingPermission,
    latestPlanDecisionSurface,
    planDecisionApproval: latestPlanDecisionSurface?.approval,
    planDecisionIsCurrent: !Number.isNaN(planDecisionAt) && planDecisionAt >= latestUserAt,
    panelProgress: panelProgressEvent ? progressSurfaceFromEvent(panelProgressEvent) : undefined,
    providerSubagents: providerSubagentsFromEvents(settledTranscriptEvents),
    spawnedThreads: spawnedThreadsFromSources(acpChildren, [...persistedEvents, ...liveEvents]),
  }
}
