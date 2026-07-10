import { ChevronDown } from 'lucide-react'
import { AnimatePresence, motion, type Variants, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'

import type { GoalEvent } from '@/lib/api/types'

const numberFormatter = new Intl.NumberFormat()
type NumberRoll = { direction: number; delay: number }

const numberRoll: Variants = {
  enter: ({ direction }: NumberRoll) => ({ opacity: 0, y: `${direction * 80}%`, filter: 'blur(2px)' }),
  center: ({ delay }: NumberRoll) => ({
    opacity: 1,
    y: '0%',
    filter: 'blur(0px)',
    transition: { duration: 0.7, delay, ease: [0.2, 0, 0, 1] },
  }),
  exit: ({ direction, delay }: NumberRoll) => ({
    opacity: 0,
    y: `${direction * -65}%`,
    filter: 'blur(2px)',
    transition: { duration: 0.5, delay: delay / 2, ease: 'easeIn' },
  }),
}

export function GoalStatusBar({ goal }: { goal?: GoalEvent }) {
  const [expanded, setExpanded] = useState(false)
  const reduceMotion = useReducedMotion()

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
        <motion.div
          initial={false}
          animate={{ height: expanded ? 'auto' : 20 }}
          transition={reduceMotion ? { duration: 0 } : { duration: 0.3, ease: [0.2, 0, 0, 1] }}
          className="min-w-0 flex-1 overflow-hidden break-words leading-5"
        >
          <span className="mr-2 font-medium text-primary-strong">{label}</span>
          {objective ? <span className="text-ink-2">{objective}</span> : null}
        </motion.div>
        {objective ? (
          <ChevronDown
            className={`size-4 shrink-0 text-primary-strong/70 transition-transform ${reduceMotion ? 'duration-0' : 'duration-300 ease-out'} ${expanded ? 'rotate-180' : ''}`}
            aria-hidden="true"
          />
        ) : null}
      </button>
      <AnimatePresence initial={false}>
        {expanded ? (
          <motion.div
            initial={reduceMotion ? { opacity: 0 } : { height: 0, opacity: 0 }}
            animate={reduceMotion ? { opacity: 1 } : { height: 'auto', opacity: 1 }}
            exit={reduceMotion ? { opacity: 0 } : { height: 0, opacity: 0 }}
            transition={reduceMotion ? { duration: 0.12 } : { duration: 0.3, ease: [0.2, 0, 0, 1] }}
            className="overflow-hidden"
          >
            <GoalTokens goal={goal} progress={tokenProgress} />
          </motion.div>
        ) : null}
      </AnimatePresence>
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
  const characters = [...formatted]
  const direction = value < previous.current ? -1 : 1

  useEffect(() => {
    previous.current = value
  }, [value])

  if (reduceMotion) return formatted

  return (
    <span className="relative inline-grid align-bottom tabular-nums">
      <span className="sr-only">{formatted}</span>
      <span className="inline-flex" aria-hidden="true">
        {characters.map((character, index) => {
          const place = characters.length - index
          const delay = characters.length > 1 ? ((place - 1) / (characters.length - 1)) * 0.3 : 0
          if (character < '0' || character > '9') return <span key={place}>{character}</span>
          return (
            <RollingDigit
              key={place}
              value={value}
              character={character}
              direction={direction}
              delay={delay}
            />
          )
        })}
      </span>
    </span>
  )
}

function RollingDigit({
  value,
  character,
  direction,
  delay,
}: {
  value: number
  character: string
  direction: number
  delay: number
}) {
  const roll = { direction, delay }
  return (
    <span className="relative inline-grid overflow-hidden align-bottom">
      <AnimatePresence initial={false} custom={roll}>
        <motion.span
          key={value}
          className="col-start-1 row-start-1 block"
          custom={roll}
          variants={numberRoll}
          initial="enter"
          animate="center"
          exit="exit"
        >
          {character}
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
