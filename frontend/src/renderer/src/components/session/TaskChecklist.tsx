import { Check, ChevronDown, Circle, LoaderCircle } from 'lucide-react'
import { memo, useEffect, useRef, useState } from 'react'
import { taskStepState, type TaskStepState, type TaskSurface } from '@/lib/taskSurface'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { MessageMarkdown } from './MessageMarkdown'

export function TaskStepIcon({ state, active }: { state: TaskStepState; active: boolean }) {
  switch (state) {
    case 'active':
      return (
        <LoaderCircle
          size={14}
          className={`text-running ${active ? 'animate-spin' : ''}`}
          aria-hidden
        />
      )
    default:
      return <Circle size={14} className="text-ink-3" aria-hidden />
  }
}

export const TaskChecklist = memo(function TaskChecklist({
  surface,
  active = false,
  onApprovePlan,
}: {
  surface: TaskSurface
  active?: boolean
  onApprovePlan?: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [overflowing, setOverflowing] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const { title, explanation, entries, strikeCompleted } = surface

  useEffect(() => {
    const el = contentRef.current
    if (!el) return

    const measure = () => {
      setOverflowing(el.scrollHeight > el.clientHeight + 2)
    }
    measure()

    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [entries, expanded, explanation])

  const showExpandControl = expanded || overflowing
  const taskEntries = entries ?? []
  const explanationText = explanation?.trim() ?? ''
  const stepStates = taskEntries.map(taskStepState)
  const showSteps = stepStates.some(Boolean)
  const completedCount = stepStates.filter((state) => state === 'completed').length

  return (
    <div className="rounded-card border border-border bg-surface px-3 py-2.5">
      <div className="mb-2 flex items-center justify-between gap-3">
        <p className="text-[11px] font-medium tracking-wide text-ink-2 uppercase">
          {title}
          {showSteps ? (
            <span className="ml-2 font-mono normal-case tracking-normal">
              {completedCount}/{taskEntries.length}
            </span>
          ) : null}
        </p>
        {surface.awaitingApproval && onApprovePlan ? (
          <Button variant="primary" size="sm" onClick={onApprovePlan}>
            <Check size={13} />
            Approve plan
          </Button>
        ) : null}
      </div>
      <div
        ref={contentRef}
        className={`relative ${expanded ? '' : 'max-h-[340px] overflow-hidden'}`}
      >
        {explanationText ? (
          <div className="mb-2 text-sm text-ink">
            <MessageMarkdown text={explanationText} />
          </div>
        ) : null}
        {taskEntries.length ? (
          <ul className="flex flex-col gap-2.5">
            {taskEntries.map((entry, index) => {
              const state = stepStates[index]
              const done = state === 'completed'
              return (
                <li
                  key={`${entry.content}-${index}`}
                  className="flex min-w-0 items-start gap-2 text-sm text-ink-2"
                >
                  {showSteps ? (
                    <span className="mt-[3px] shrink-0" title={state}>
                      <TaskStepIcon state={state ?? 'pending'} active={active} />
                    </span>
                  ) : null}
                  <div
                    className={`min-w-0 flex-1 ${done ? `opacity-50 ${strikeCompleted ? 'line-through' : ''}` : ''}`}
                  >
                    {surface.kind === 'progress' ? (
                      entry.content
                    ) : (
                      <MessageMarkdown text={entry.content} />
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        ) : explanationText ? null : (
          <p className="text-sm italic text-ink-3">(no steps provided)</p>
        )}
        {!expanded && overflowing ? (
          <div
            className="pointer-events-none absolute inset-x-0 bottom-0 h-20 bg-gradient-to-b from-transparent via-surface/85 to-surface"
            aria-hidden
          />
        ) : null}
      </div>
      {/* Centered chevron-in-a-circle is the only expand affordance; it lifts
          onto the fade when collapsed and flips to point up when open. */}
      {showExpandControl ? (
        <div className={`relative z-10 flex justify-center ${expanded ? 'mt-1.5' : '-mt-3.5'}`}>
          <IconButton
            variant="ghost"
            size="md"
            aria-expanded={expanded}
            aria-label={expanded ? `Collapse ${title}` : `Expand ${title}`}
            title={expanded ? `Collapse ${title}` : `Expand ${title}`}
            className="border border-border bg-surface shadow-sm"
            onClick={() => setExpanded((value) => !value)}
          >
            <ChevronDown
              size={15}
              className={`transition-transform duration-200 ease-out ${expanded ? 'rotate-180' : ''}`}
              aria-hidden
            />
          </IconButton>
        </div>
      ) : null}
    </div>
  )
})
