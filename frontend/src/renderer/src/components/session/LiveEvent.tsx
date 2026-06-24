import { memo } from 'react'
import type { ACPPermission, SessionEvent } from '@/lib/api/types'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { isParentChildACPEvent } from '@/lib/sessionEvents'
import { relativeTime } from '@/lib/format/time'
import { taskSurfaceFromEvent } from '@/lib/taskSurface'
import { ArtifactBlock } from './ArtifactBlock'
import { AssistantMarkdown } from './AssistantMarkdown'
import { LoopCreatedCard } from './LoopCreatedCard'
import { TaskChecklist } from './TaskChecklist'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolSummary } from './ToolDisclosure'
import { PermissionCard } from './TranscriptPermissions'

export const LiveEvent = memo(function LiveEvent({
  event,
  showHeader,
  working = false,
  permissionResolution,
  showTaskSurface,
  onApprovePlan,
  onArtifactPrompt,
}: {
  event: SessionEvent
  showHeader: boolean
  working?: boolean
  permissionResolution?: ACPPermission
  showTaskSurface?: boolean
  onApprovePlan?: () => void
  onArtifactPrompt?: (text: string) => void
}) {
  const eventTaskSurface = taskSurfaceFromEvent(event)
  const taskSurface = showTaskSurface ? eventTaskSurface : undefined
  const parentChild = isParentChildACPEvent(event)
  const artifact = event.type === 'artifact' ? event.artifact : undefined
  const loopCreated = event.type === 'loop_created' ? event.loop_created : undefined
  return (
    <div className="flex min-w-0 max-w-[var(--prose-max)] flex-col gap-2">
      {event.acp && showHeader ? (
        <p className="text-[12px] text-ink-3">
          {hasAgentLogo(event.acp.agent) ? (
            <AgentLogo
              agent={event.acp.agent}
              size={12}
              className="inline-block translate-y-[2px] text-ink-2"
            />
          ) : (
            <span className="font-mono">{event.acp.agent}</span>
          )}
          {event.acp.title ? ` · ${event.acp.title}` : ''} · {relativeTime(event.at)}
        </p>
      ) : null}
      {event.acp?.thought ? <ThinkingBlock text={event.acp.thought} /> : null}
      {artifact ? (
        <ArtifactBlock artifact={artifact} onSendPrompt={onArtifactPrompt} />
      ) : null}
      {loopCreated ? <LoopCreatedCard loop={loopCreated} /> : null}
      {event.content && !artifact ? <AssistantMarkdown text={event.content} /> : null}
      {!parentChild ? <ToolSummary calls={event.acp?.tool_calls} active={working} /> : null}
      {event.permission ? (
        <PermissionCard event={event} resolution={permissionResolution} />
      ) : null}
      {taskSurface ? (
        <TaskChecklist surface={taskSurface} active={working} onApprovePlan={onApprovePlan} />
      ) : null}
    </div>
  )
})
