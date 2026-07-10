import { ChevronDown } from 'lucide-react'
import { AnimatePresence, motion, type Variants, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'

import type { GoalEvent } from '@/lib/api/types'

const numberFormatter = new Intl.NumberFormat()
const numberRoll: Variants = {
  enter: (direction: number) => ({ opacity: 0, y: `${direction * 80}%`, filter: 'blur(2px)' }),
  center: {
    opacity: 1,
    y: '0%',
    filter: 'blur(0px)',
    transition: { duration: 0.28, ease: [0.2, 0, 0, 1] },
  },
  exit: (direction: number) => ({
    opacity: 0,
    y: `${direction * -65}%`,
    filter: 'blur(2px)',
    transition: { duration: 0.18, ease: 'easeIn' },
  }),
}

export function GoalStatusBar({ goal }: { goal?: GoalEvent }) {
  const [expanded, setExpanded] = useState(false)

  if (!goal) return null

  const objective = goal?.objective?.trim()
  const label = goalStatusLabel(goal?.status)
  const tokenProgress = goalTokenProgress(goal)

  return (
    <div className="mb-2 rounded-[8px] bg-primary-soft/70 px-3 py-1.5 text-[13px] shadow-sm ring-1 ring-primary/20">
      <button
        type="button"
        className="flex w-full min-w-0 items-center gap-3 text-left transition-transform duration-150 active:scale-[0.99]"
        aria-expanded={expanded}
        onClick={() => setExpanded((value) => !value)}
      >
        <div className={`min-w-0 flex-1 leading-5 ${expanded ? 'break-words' : 'truncate'}`}>
          <span className="mr-2 font-medium text-primary-strong">{label}</span>
          {objective ? <span className="text-ink-2">{objective}</span> : null}
        </div>
        {objective ? (
          <ChevronDown
            className={`size-4 shrink-0 text-primary-strong/70 transition-transform duration-150 ${expanded ? 'rotate-180' : ''}`}
            aria-hidden="true"
          />
        ) : null}
      </button>
      {expanded ? <GoalTokens goal={goal} progress={tokenProgress} /> : null}
    </div>
  )
}

function GoalTokens({
  goal,
  progress,
}: {
  goal: GoalEvent
  progress?: { used: number; budget: number; percent: number }
}) {
  if (progress) {
    return (
      <div className="mt-2 flex items-center gap-2">
        <div
          className="h-1.5 flex-1 overflow-hidden rounded-full bg-bg/80 shadow-inner"
          aria-label={`${numberFormatter.format(progress.used)} of ${numberFormatter.format(progress.budget)} goal tokens used`}
        >
          <div
            className="h-full rounded-full bg-primary transition-[width] duration-150"
            style={{ width: `${progress.percent}%` }}
          />
        </div>
        <span className="shrink-0 text-[12px] tabular-nums text-ink-3">
          <RollingNumber value={progress.used} /> / <RollingNumber value={progress.budget} />
        </span>
      </div>
    )
  }
  if (goal.tokens_used == null) return null
  return (
    <div className="mt-2 text-[12px] tabular-nums text-ink-3">
      <RollingNumber value={goal.tokens_used} /> goal tokens
    </div>
  )
}

function RollingNumber({ value }: { value: number }) {
  const previous = useRef(value)
  const reduceMotion = useReducedMotion()
  const formatted = numberFormatter.format(value)
  const direction = value < previous.current ? -1 : 1

  useEffect(() => {
    previous.current = value
  }, [value])

  if (reduceMotion) return formatted

  return (
    <span className="relative inline-grid overflow-hidden align-bottom tabular-nums">
      <span className="sr-only">{formatted}</span>
      <AnimatePresence initial={false} custom={direction}>
        <motion.span
          key={value}
          aria-hidden="true"
          className="col-start-1 row-start-1 block"
          custom={direction}
          variants={numberRoll}
          initial="enter"
          animate="center"
          exit="exit"
        >
          {formatted}
        </motion.span>
      </AnimatePresence>
    </span>
  )
}

function goalStatusLabel(status?: string): string {
  switch (status) {
    case 'requested':
      return 'Goal requested'
    case 'active':
      return 'Goal'
    case 'complete':
      return 'Goal complete'
    case 'blocked':
      return 'Goal blocked'
    case 'budgetLimited':
      return 'Goal budget limited'
    case 'usageLimited':
      return 'Goal usage limited'
    default:
      return status ? `Goal ${status}` : 'Goal'
  }
}

function goalTokenProgress(goal?: GoalEvent): { used: number; budget: number; percent: number } | undefined {
  if (goal?.token_budget == null) return undefined
  const used = Math.max(0, goal.tokens_used ?? 0)
  const budget = Math.max(0, goal.token_budget)
  if (budget === 0) return { used, budget, percent: used > 0 ? 100 : 0 }
  return { used, budget, percent: Math.min(100, (used / budget) * 100) }
}
