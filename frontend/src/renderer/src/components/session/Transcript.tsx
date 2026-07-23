import { ChevronDown } from 'lucide-react'
import { memo, useEffect, useMemo, useState, type ReactNode } from 'react'
import type { ChatMessage, SessionEvent } from '@/lib/api/types'
import { Button } from '@/components/ui/Button'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { taskSurfaceFromEvent } from '@/lib/taskSurface'
import {
  buildTimeline,
  classifyTurnItems,
  stableEventKey,
  type TimelineItem,
} from './timeline'
import { Bubble } from './Bubble'
import { LiveEvent } from './LiveEvent'
import type { SessionErrorAction } from './SessionErrorNotice'
import { ToolDisclosure } from './ToolDisclosure'

const INITIAL_VISIBLE_TURNS = 14
const VISIBLE_TURN_BATCH = 24
const INITIAL_VISIBLE_ITEMS = 90
const VISIBLE_ITEM_BATCH = 120

type RenderOptions = {
  showAssistantCopy?: boolean
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
        className="self-start font-medium"
      />
      <Collapse open={effectiveOpen} className="w-full">
        <div className="flex flex-col gap-2 pt-3">{items.map((item) => render(item))}</div>
      </Collapse>
    </div>
  )
}

function EarlierHistoryButton({
  hiddenCount,
  unit,
  hasMore,
  loading,
  onClick,
}: {
  hiddenCount: number
  unit: string
  hasMore?: boolean
  loading?: boolean
  onClick: () => void
}) {
  if (hiddenCount <= 0 && !hasMore) return null
  return (
    <div className="flex justify-center">
      <Button
        variant="ghost"
        size="sm"
        className="border border-border bg-bg/90"
        title={hiddenCount > 0 ? `${hiddenCount} earlier ${unit}` : 'Load earlier history'}
        aria-expanded={false}
        disabled={loading}
        onClick={onClick}
      >
        <ChevronDown size={13} className="rotate-180" aria-hidden />
        {loading ? 'Loading…' : 'Earlier history'}
      </Button>
    </div>
  )
}

// Result cards read as a turn's outcome, so they anchor to the end of the turn
// rather than folding into its work.
function isResultCard(item: TimelineItem): boolean {
  return item.kind === 'event' && item.event.type === 'loop_created'
}

function trailingErrorEventIndex(chronological: TimelineItem[], anchored: TimelineItem[]): number | undefined {
  const lastItem = (anchored.length ? anchored : chronological).at(-1)
  return lastItem?.kind === 'event' && lastItem.event.acp?.error ? lastItem.eventIndex : undefined
}

export const Transcript = memo(function Transcript({
  messages,
  events,
  sessionId,
  attachmentSessionId = sessionId,
  groupTurns = false,
  working = false,
  findActive = false,
  highlightedSeq,
  tail,
  errorAction,
  onApprovePlan,
  onArtifactPrompt,
  hasEarlierHistory = false,
  loadingEarlierHistory = false,
  onLoadEarlierHistory,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
  sessionId?: string
  attachmentSessionId?: string
  groupTurns?: boolean
  working?: boolean
  findActive?: boolean
  highlightedSeq?: number
  // in-flight live exchange, rendered between history and anchored live state
  tail?: ReactNode
  errorAction?: SessionErrorAction
  onApprovePlan?: () => void
  onArtifactPrompt?: (text: string) => void
  hasEarlierHistory?: boolean
  loadingEarlierHistory?: boolean
  onLoadEarlierHistory?: () => Promise<boolean>
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

  useEffect(() => {
    if (highlightedSeq) setVisibleHistoryCount(historyCount)
  }, [highlightedSeq, historyCount])

  const historyStart = findActive ? 0 : Math.max(0, historyCount - visibleHistoryCount)
  const hiddenHistoryCount = historyStart
  const visibleChronological = chronological.slice(historyStart)
  const visibleTurns = turns.slice(historyStart)
  const errorActionEventIndex = errorAction ? trailingErrorEventIndex(chronological, anchored) : undefined

  const revealEarlierHistory = () => {
    if (hiddenHistoryCount > 0) {
      setVisibleHistoryCount((count) => Math.min(historyCount, count + historyBatchSize))
      return
    }
    if (!onLoadEarlierHistory) return
    void onLoadEarlierHistory().then((loaded) => {
      if (loaded) setVisibleHistoryCount(Number.MAX_SAFE_INTEGER)
    })
  }

  const renderItem = (item: TimelineItem, options: RenderOptions = {}): ReactNode => {
    const showAssistantCopy = options.showAssistantCopy ?? true
    switch (item.kind) {
      case 'message':
        return (
          <div
            key={`message-${item.message.seq}`}
            data-message-seq={item.message.seq}
            className={`scroll-mt-24 rounded-card transition-[outline-color,box-shadow] duration-200 ${
              groupTurns ? '' : 'my-1.5'
            } ${
              highlightedSeq === item.message.seq
                ? 'outline-2 outline-offset-4 outline-primary/50 shadow-[0_0_0_8px_color-mix(in_oklab,var(--color-primary)_10%,transparent)]'
                : 'outline-2 outline-offset-4 outline-transparent'
            }`}
          >
            <Bubble
              message={item.message}
              showAssistantCopy={showAssistantCopy}
              onArtifactPrompt={onArtifactPrompt}
              attachmentSessionId={attachmentSessionId}
            />
          </div>
        )
      case 'tools':
        return (
          <ToolDisclosure key={item.key} calls={item.calls} active={working} />
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
      <div className="flex flex-col gap-2">
        <EarlierHistoryButton
          hiddenCount={hiddenHistoryCount}
          unit="history items"
          hasMore={hasEarlierHistory}
          loading={loadingEarlierHistory}
          onClick={revealEarlierHistory}
        />
        {visibleChronological.map((item) => renderItem(item))}
        {tail}
        {anchored.map((item) => renderItem(item))}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <EarlierHistoryButton
        hiddenCount={hiddenHistoryCount}
        unit="turns"
        hasMore={hasEarlierHistory}
        loading={loadingEarlierHistory}
        onClick={revealEarlierHistory}
      />
      {visibleTurns.map((turn, visibleTurnIndex) => {
        const turnIndex = historyStart + visibleTurnIndex
        const active = working && turnIndex === turns.length - 1
        // A created-loop card reads as the turn's outcome, so pull it out of the
        // flow and append it at the end rather than folding it into the work.
        const resultCards = turn.items.filter(isResultCard)
        const flow = turn.items.filter((item) => !isResultCard(item))
        const sections: ReactNode[] = []
        if (turn.opener) sections.push(renderItem(turn.opener))
        if (active) {
          // Live turn: stream items in order. Answer-vs-narration classification
          // isn't stable until the turn completes, so nothing folds yet.
          flow.forEach((item) => sections.push(renderItem(item)))
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
                key={`work-${turnIndex}`}
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
          <div key={`turn-${turnIndex}`} className={`flex flex-col ${active ? 'gap-2' : 'gap-4'}`}>
            {sections}
          </div>
        )
      })}
      {tail}
      {anchored.map((item) => renderItem(item))}
    </div>
  )
})
