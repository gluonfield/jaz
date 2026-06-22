import { ChevronDown, ChevronRight } from 'lucide-react'
import { memo, useEffect, useMemo, useState, type ReactNode } from 'react'
import type { ChatMessage, SessionEvent } from '@/lib/api/types'
import { Button } from '@/components/ui/Button'
import { taskSurfaceFromEvent } from '@/lib/taskSurface'
import {
  buildTimeline,
  isCollapsibleWork,
  isSpawnedAgentEvent,
  stableEventKey,
  type TimelineItem,
} from './timeline'
import { Bubble } from './Bubble'
import { LiveEvent } from './LiveEvent'
import { ToolDisclosure, toolRunLabel } from './ToolDisclosure'

const INITIAL_VISIBLE_TURNS = 14
const VISIBLE_TURN_BATCH = 24
const INITIAL_VISIBLE_ITEMS = 90
const VISIBLE_ITEM_BATCH = 120

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
    <div className="flex flex-col gap-5">
      <button
        type="button"
        aria-expanded={effectiveOpen}
        onClick={() => setOpen((value) => !value)}
        className="inline-flex min-h-7 items-center gap-1.5 self-start rounded-full px-1 text-left text-[12px] font-medium text-ink-3 transition-colors hover:text-ink"
      >
        <ChevronRight
          size={12}
          className={`shrink-0 transition-transform ${effectiveOpen ? 'rotate-90' : ''}`}
          aria-hidden
        />
        Worked for {formatDuration(durationMs)}
      </button>
      {effectiveOpen ? (
        <div className="flex flex-col gap-5 border-l border-border pl-4">
          {items.map((item) => render(item))}
        </div>
      ) : null}
    </div>
  )
}

function EarlierHistoryButton({
  hiddenCount,
  unit,
  onClick,
}: {
  hiddenCount: number
  unit: string
  onClick: () => void
}) {
  if (hiddenCount <= 0) return null
  return (
    <div className="flex justify-center">
      <Button
        variant="ghost"
        size="sm"
        className="border border-border bg-bg/90"
        title={`${hiddenCount} earlier ${unit}`}
        onClick={onClick}
      >
        <ChevronDown size={13} className="rotate-180" aria-hidden />
        Earlier history
      </Button>
    </div>
  )
}

// Result cards (a created loop, a spawned agent's run) read as a turn's outcome,
// so they anchor to the end of the turn rather than folding into its work.
function isResultCard(item: TimelineItem): boolean {
  return (
    item.kind === 'event' &&
    (item.event.type === 'loop_created' || isSpawnedAgentEvent(item.event))
  )
}

export const Transcript = memo(function Transcript({
  messages,
  events,
  sessionId,
  groupTurns = false,
  working = false,
  findActive = false,
  highlightedSeq,
  tail,
  onApprovePlan,
  onArtifactPrompt,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
  sessionId?: string
  groupTurns?: boolean
  working?: boolean
  findActive?: boolean
  highlightedSeq?: number
  // in-flight live exchange, rendered between history and anchored live state
  tail?: ReactNode
  onApprovePlan?: () => void
  onArtifactPrompt?: (text: string) => void
}) {
  const {
    chronological,
    anchored,
    turns,
    permissionResolutions,
    latestTaskSurfaceEvent,
    pendingPermissionIds,
  } = useMemo(
    () => buildTimeline(messages, events, sessionId, groupTurns),
    [messages, events, sessionId, groupTurns],
  )
  const [visibleHistoryCount, setVisibleHistoryCount] = useState(
    groupTurns ? INITIAL_VISIBLE_TURNS : INITIAL_VISIBLE_ITEMS,
  )
  const historyCount = groupTurns ? turns.length : chronological.length
  const baselineVisibleHistory = groupTurns ? INITIAL_VISIBLE_TURNS : INITIAL_VISIBLE_ITEMS
  const historyBatchSize = groupTurns ? VISIBLE_TURN_BATCH : VISIBLE_ITEM_BATCH

  useEffect(() => {
    setVisibleHistoryCount((count) =>
      Math.min(historyCount, Math.max(count, baselineVisibleHistory)),
    )
  }, [baselineVisibleHistory, historyCount])

  const historyStart = findActive ? 0 : Math.max(0, historyCount - visibleHistoryCount)
  const hiddenHistoryCount = historyStart
  const visibleChronological = chronological.slice(historyStart)
  const visibleTurns = turns.slice(historyStart)

  const renderItem = (item: TimelineItem): ReactNode => {
    switch (item.kind) {
      case 'message':
        return (
          <div
            key={`message-${item.message.seq}`}
            data-message-seq={item.message.seq}
            className={`scroll-mt-24 rounded-card transition-[outline-color,box-shadow] duration-200 ${
              highlightedSeq === item.message.seq
                ? 'outline-2 outline-offset-4 outline-primary/50 shadow-[0_0_0_8px_color-mix(in_oklab,var(--color-primary)_10%,transparent)]'
                : 'outline-2 outline-offset-4 outline-transparent'
            }`}
          >
            <Bubble
              message={item.message}
              onArtifactPrompt={onArtifactPrompt}
            />
          </div>
        )
      case 'tools':
        return (
          <ToolDisclosure
            key={item.key}
            label={toolRunLabel(item.calls)}
            calls={item.calls}
            active={working}
          />
        )
      case 'event': {
        const taskSurface = taskSurfaceFromEvent(item.event)
        return (
          <LiveEvent
            key={`event-${stableEventKey(item.event, item.eventIndex)}`}
            event={item.event}
            showHeader={item.showHeader}
            working={working}
            showTaskSurface={
              Boolean(
                taskSurface &&
                  (!item.event.acp ||
                    latestTaskSurfaceEvent.get(item.event.acp.id) === item.eventIndex),
              )
            }
            onApprovePlan={onApprovePlan}
            onArtifactPrompt={onArtifactPrompt}
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
      <div className="flex flex-col gap-5">
        <EarlierHistoryButton
          hiddenCount={hiddenHistoryCount}
          unit="history items"
          onClick={() =>
            setVisibleHistoryCount((count) => Math.min(historyCount, count + historyBatchSize))
          }
        />
        {visibleChronological.map((item) => renderItem(item))}
        {tail}
        {anchored.map((item) => renderItem(item))}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <EarlierHistoryButton
        hiddenCount={hiddenHistoryCount}
        unit="turns"
        onClick={() =>
          setVisibleHistoryCount((count) => Math.min(historyCount, count + historyBatchSize))
        }
      />
      {visibleTurns.map((turn, visibleTurnIndex) => {
        const turnIndex = historyStart + visibleTurnIndex
        const active = working && turnIndex === turns.length - 1
        // Pull result cards out of the chronological flow so the work before and
        // after the tool call folds into a single "Worked for", and append them
        // as the turn's outcome below.
        const resultCards = turn.items.filter(isResultCard)
        const flow = turn.items.filter((item) => !isResultCard(item))
        const lastContentIndex = flow.findLastIndex(
          (item) => item.kind === 'event' && Boolean(item.event.content?.trim()),
        )
        const sections: ReactNode[] = []
        if (turn.opener) sections.push(renderItem(turn.opener))
        let work: TimelineItem[] = []
        const flushWork = () => {
          if (!work.length) return
          const batch = work
          work = []
          const durationMs =
            batch[batch.length - 1].at - (turn.opener?.at ?? batch[0].at)
          sections.push(
            <WorkSection
              key={`work-${turnIndex}-${sections.length}`}
              items={batch}
              durationMs={durationMs}
              defaultOpen={false}
              findActive={findActive}
              render={renderItem}
            />,
          )
        }
        flow.forEach((item, index) => {
          const collapsible =
            !active &&
            index < lastContentIndex &&
            isCollapsibleWork(item, pendingPermissionIds, latestTaskSurfaceEvent)
          if (collapsible) {
            work.push(item)
            return
          }
          flushWork()
          sections.push(renderItem(item))
        })
        flushWork()
        resultCards.forEach((item) => sections.push(renderItem(item)))
        return (
          <div key={`turn-${turnIndex}`} className="flex flex-col gap-5">
            {sections}
          </div>
        )
      })}
      {tail}
      {anchored.map((item) => renderItem(item))}
    </div>
  )
})
