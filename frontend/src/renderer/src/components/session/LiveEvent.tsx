import { memo } from 'react'
import type { ACPPermission, SessionEvent } from '@/lib/api/types'
import { isParentChildACPEvent } from '@/lib/sessionEvents'
import { taskSurfaceFromEvent } from '@/lib/taskSurface'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { Bubble } from '@/components/session/Bubble'
import { ACPEventHeader } from './ACPEventHeader'
import { ArtifactBlock } from './ArtifactBlock'
import { AssistantMarkdown } from './AssistantMarkdown'
import { LoopCreatedCard } from './LoopCreatedCard'
import { SessionErrorNotice, type SessionErrorAction } from './SessionErrorNotice'
import { TaskChecklist } from './TaskChecklist'
import { ToolDisclosure } from './ToolDisclosure'
import { PermissionCard } from './TranscriptPermissions'

export const LiveEvent = memo(function LiveEvent({
  event,
  showHeader,
  working = false,
  findActive = false,
  showCopy = true,
  permissionResolution,
  showTaskSurface,
  onApprovePlan,
  onArtifactPrompt,
  errorAction,
}: {
  event: SessionEvent
  showHeader: boolean
  working?: boolean
  findActive?: boolean
  showCopy?: boolean
  permissionResolution?: ACPPermission
  showTaskSurface?: boolean
  onApprovePlan?: () => void
  onArtifactPrompt?: (text: string) => void
  errorAction?: SessionErrorAction
}) {
  if (event.voice) {
    return <Bubble message={{ seq: event.seq ?? 0, role: event.voice.role, content: event.voice.text, blocks: [], created_at: event.voice.at }} showAssistantCopy={showCopy} />
  }
  const eventTaskSurface = taskSurfaceFromEvent(event)
  const taskSurface = showTaskSurface ? eventTaskSurface : undefined
  const parentChild = isParentChildACPEvent(event)
  const toolCalls = parentChild ? undefined : event.acp?.tool_calls
  const artifact = event.type === 'artifact' ? event.artifact : undefined
  const loopCreated = event.type === 'loop_created' ? event.loop_created : undefined
  return (
    <div className="flex min-w-0 max-w-[var(--prose-max)] flex-col gap-2">
      {showHeader && event.acp ? (
        <ACPEventHeader agent={event.acp.agent} title={event.acp.title} at={event.at} />
      ) : null}
      {event.acp?.thought ? <ThinkingBlock text={event.acp.thought} findActive={findActive} /> : null}
      {artifact ? (
        <ArtifactBlock artifact={artifact} onSendPrompt={onArtifactPrompt} />
      ) : null}
      {loopCreated ? <LoopCreatedCard loop={loopCreated} /> : null}
      {event.content && !artifact ? (
        <AssistantMarkdown text={event.content} createdAt={event.at} showCopy={showCopy} />
      ) : null}
      {event.acp?.error ? <SessionErrorNotice message={event.acp.error} action={errorAction} /> : null}
      {toolCalls?.length ? <ToolDisclosure calls={toolCalls} active={working} /> : null}
      {event.permission ? (
        <PermissionCard event={event} resolution={permissionResolution} />
      ) : null}
      {taskSurface ? (
        <TaskChecklist surface={taskSurface} active={working} onApprovePlan={onApprovePlan} />
      ) : null}
    </div>
  )
})
