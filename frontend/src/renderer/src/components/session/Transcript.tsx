import { memo, useMemo, useState, type ReactNode, type RefObject } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { ChatMessage, MCPEntrypoint, SessionEvent } from '@/lib/api/types'
import { mcpEntrypointsQuery } from '@/lib/api/mcp'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { taskSurfaceFromEvent } from '@/lib/taskSurface'
import {
  buildTimeline,
  classifyTurnItems,
  stableEventKey,
  type TimelineItem,
} from './timeline'
import { useHistoryWindow } from '@/components/session/useHistoryWindow'
import type { HistoryPaging } from '@/lib/hooks/useSessionHistory'
import { SpokenReply } from '@/components/session/SpokenReply'
import { ActivityBlock } from './ActivityBlock'
import { Bubble } from './Bubble'
import { LiveEvent } from './LiveEvent'
import type { SessionErrorAction } from './SessionErrorNotice'

const INITIAL_VISIBLE_TURNS = 14
const VISIBLE_TURN_BATCH = 24
const INITIAL_VISIBLE_ITEMS = 90
const VISIBLE_ITEM_BATCH = 120
const NO_ENTRYPOINTS: MCPEntrypoint[] = []

type RenderOptions = {
  showAssistantCopy?: boolean
  activityActive?: boolean
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(1, Math.round(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours) return `${hours}h ${minutes}m`
  if (minutes) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

function WorkSection({
  items,
  durationMs,
  defaultOpen,
  findActive = false,
  render,
}: {
  items: TimelineItem[]
  durationMs: number
  defaultOpen: boolean
  findActive?: boolean
  render: (item: TimelineItem) => ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  const effectiveOpen = open || findActive

  return (
    <div className="flex flex-col">
      <DisclosureTrigger
        label={`Worked for ${formatDuration(durationMs)}`}
        open={effectiveOpen}
        onClick={() => setOpen((value) => !value)}
        className="self-start font-medium tabular-nums"
      />
      <Collapse open={effectiveOpen} className="w-full">
        <div className="flex flex-col gap-3 pt-3">{items.map((item) => render(item))}</div>
      </Collapse>
    </div>
  )
}

function itemKey(item: TimelineItem): string {
  switch (item.kind) {
    case 'message':
      return `message-${item.message.seq}`
    case 'activity':
      return item.key
    case 'event':
      return `event-${stableEventKey(item.event, item.eventIndex)}`
  }
}

// Result cards and system rows never fold into a turn's work: they anchor to
// the end of the turn, which also lands a row that precedes the next turn
// directly above it.
const RESULT_EVENTS = new Set(['loop_created', 'mcp_app', 'bot_activity'])

function isResultCard(item: TimelineItem): boolean {
  return item.kind === 'event' && RESULT_EVENTS.has(item.event.type)
}

function trailingErrorEventIndex(chronological: TimelineItem[], anchored: TimelineItem[]): number | undefined {
  const lastItem = (anchored.length ? anchored : chronological).at(-1)
  return lastItem?.kind === 'event' && lastItem.event.acp?.error ? lastItem.eventIndex : undefined
}

export const Transcript = memo(function Transcript({
  messages,
  events,
  scrollRef,
  sessionId,
  attachmentSessionId = sessionId,
  groupTurns = false,
  working = false,
  findActive = false,
  revealSeq,
  tail,
  errorAction,
  onApprovePlan,
  onArtifactPrompt,
  paging,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
  scrollRef: RefObject<HTMLDivElement | null>
  sessionId?: string
  attachmentSessionId?: string
  groupTurns?: boolean
  working?: boolean
  findActive?: boolean
  // A jump target outside the rendered window: drop the window so it exists.
  revealSeq?: number
  // in-flight live exchange, rendered between history and anchored live state
  tail?: ReactNode
  errorAction?: SessionErrorAction
  onApprovePlan?: () => void
  onArtifactPrompt?: (text: string) => void
  paging?: HistoryPaging
}) {
  const entrypoints = useQuery(mcpEntrypointsQuery).data ?? NO_ENTRYPOINTS
  const {
    chronological,
    anchored,
    turns,
    permissionResolutions,
    latestTaskSurfaceEvent,
    pendingPermissionIds,
  } = useMemo(
    () => buildTimeline(messages, events, sessionId, groupTurns, entrypoints),
    [messages, events, sessionId, groupTurns, entrypoints],
  )
  const firstItemAt = (index: number) =>
    groupTurns ? turns[index]?.opener ?? turns[index]?.items[0] : chronological[index]
  const { start: historyStart, historyRef, sentinelRef } = useHistoryWindow({
    scrollRef,
    count: groupTurns ? turns.length : chronological.length,
    initial: groupTurns ? INITIAL_VISIBLE_TURNS : INITIAL_VISIBLE_ITEMS,
    batch: groupTurns ? VISIBLE_TURN_BATCH : VISIBLE_ITEM_BATCH,
    keyAt: (index) => {
      const item = firstItemAt(index)
      return item && itemKey(item)
    },
    paging,
    // A find or a jump target must be able to land anywhere in the history.
    showAll: findActive || Boolean(revealSeq),
  })
  const visibleChronological = chronological.slice(historyStart)
  const visibleTurns = turns.slice(historyStart)
  const errorActionEventIndex = errorAction ? trailingErrorEventIndex(chronological, anchored) : undefined

  const historySentinel = <div ref={sentinelRef} className="pointer-events-none absolute inset-x-0 top-0 h-px" aria-hidden />

  const renderItem = (item: TimelineItem, options: RenderOptions = {}): ReactNode => {
    const showAssistantCopy = options.showAssistantCopy ?? true
    switch (item.kind) {
      case 'message':
        return (
          <div
            key={itemKey(item)}
            data-message-seq={item.message.seq}
            className={`scroll-mt-24 ${groupTurns ? '' : 'my-1.5'}`}
          >
            <Bubble
              message={item.message}
              showAssistantCopy={showAssistantCopy}
              onArtifactPrompt={onArtifactPrompt}
              attachmentSessionId={attachmentSessionId}
            />
          </div>
        )
      case 'activity':
        return (
          <ActivityBlock
            key={itemKey(item)}
            entries={item.entries}
            header={item.header}
            active={options.activityActive}
            findActive={findActive}
          />
        )
      case 'event': {
        if (item.collapseVoice && item.event.voice) {
          return <SpokenReply key={itemKey(item)} voice={item.event.voice} findActive={findActive} showCopy={showAssistantCopy} />
        }
        const taskSurface = taskSurfaceFromEvent(item.event)
        return (
          <LiveEvent
            key={itemKey(item)}
            event={item.event}
            showHeader={item.showHeader}
            working={working}
            findActive={findActive}
            showTaskSurface={
              Boolean(
                taskSurface &&
                  (!item.event.acp ||
                    latestTaskSurfaceEvent.get(item.event.acp.id) === item.eventIndex),
              )
            }
            onApprovePlan={onApprovePlan}
            onArtifactPrompt={onArtifactPrompt}
            errorAction={item.eventIndex === errorActionEventIndex ? errorAction : undefined}
            showCopy={showAssistantCopy}
            permissionResolution={
              item.event.permission ? permissionResolutions.get(item.event.permission.id) : undefined
            }
          />
        )
      }
    }
  }

  if (!groupTurns) {
    return (
      <div ref={historyRef} className="relative flex flex-col gap-2" aria-busy={paging?.loading}>
        {visibleChronological.map((item, index) =>
          renderItem(item, {
            activityActive: working && index === visibleChronological.length - 1,
          }),
        )}
        {tail}
        {anchored.map((item) => renderItem(item))}
        {historySentinel}
      </div>
    )
  }

  return (
    // Turns are spaced wider than the sections inside one turn; at the same gap
    // there is nothing marking where a turn ends and the next begins.
    <div ref={historyRef} className="relative flex flex-col gap-7" aria-busy={paging?.loading}>
      {visibleTurns.map((turn, visibleTurnIndex) => {
        const turnIndex = historyStart + visibleTurnIndex
        const active = working && turnIndex === turns.length - 1
        const resultCards = turn.items.filter(isResultCard)
        const flow = turn.items.filter((item) => !isResultCard(item))
        const sections: ReactNode[] = []
        if (turn.opener) sections.push(renderItem(turn.opener))
        if (active) {
          flow.forEach((item, index) => {
            const trailing = index === flow.length - 1
            sections.push(renderItem(item, { activityActive: trailing, showAssistantCopy: false }))
          })
        } else {
          // One "Worked for" disclosure per turn holds all folded work, so a shown
          // message can't split the turn into a staircase of tiny disclosures.
          const { workItems, resultItems } = classifyTurnItems(
            flow,
            pendingPermissionIds,
            latestTaskSurfaceEvent,
          )
          if (workItems.length) {
            const durationMs =
              workItems[workItems.length - 1].at - (turn.opener?.at ?? workItems[0].at)
            sections.push(
              <WorkSection
                key="work"
                items={workItems}
                durationMs={durationMs}
                defaultOpen={false}
                findActive={findActive}
                render={(item) => renderItem(item, { showAssistantCopy: false })}
              />,
            )
          }
          resultItems.forEach((item) => sections.push(renderItem(item)))
        }
        resultCards.forEach((item) => sections.push(renderItem(item)))
        return (
          <div key={itemKey(turn.opener ?? turn.items[0])} className="flex flex-col gap-4">
            {sections}
          </div>
        )
      })}
      {tail}
      {anchored.map((item) => renderItem(item))}
      {historySentinel}
    </div>
  )
})
