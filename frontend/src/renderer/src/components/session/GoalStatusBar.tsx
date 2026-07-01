import { ChevronDown } from 'lucide-react'
import { useState } from 'react'

import type { GoalEvent } from '@/lib/api/types'

const numberFormatter = new Intl.NumberFormat()

type GoalDetail = {
  label: string
  value: string
  numeric?: boolean
}

export function GoalStatusBar({
  goal,
  running,
}: {
  goal?: GoalEvent
  running?: boolean
}) {
  const [expanded, setExpanded] = useState(false)

  if (!goal) return null

  const objective = goal?.objective?.trim()
  const label = goalStatusLabel(goal?.status)
  const tokenProgress = goalTokenProgress(goal)
  const details = goalDetails(goal)

  return (
    <div className="group/goal relative mb-2 rounded-[8px] bg-primary-soft/70 px-3 py-2 text-[13px] shadow-sm ring-1 ring-primary/20">
      <button
        type="button"
        className="flex min-h-10 w-full min-w-0 items-center gap-3 text-left transition-transform duration-150 active:scale-[0.96]"
        aria-expanded={expanded}
        onClick={() => setExpanded((value) => !value)}
      >
        <span className={`size-1.5 shrink-0 rounded-full ${goalDotClass(goal?.status, running)}`} />
        <div className="min-w-0 flex-1 leading-5">
          <div className="flex min-w-0 items-center gap-2">
            <span className="shrink-0 font-medium text-primary-strong">{label}</span>
            {objective ? <span className="min-w-0 truncate text-ink-2">{objective}</span> : null}
          </div>
        </div>
        {tokenProgress ? (
          <GoalTokenProgress progress={tokenProgress} />
        ) : goal?.tokens_used != null ? (
          <span className="shrink-0 tabular-nums text-[12px] text-ink-3">
            {numberFormatter.format(goal.tokens_used)} goal tokens
          </span>
        ) : null}
        {objective ? (
          <ChevronDown
            className={`size-4 shrink-0 text-primary-strong/70 transition-transform duration-150 ${expanded ? 'rotate-180' : ''}`}
            aria-hidden="true"
          />
        ) : null}
      </button>
      {expanded && objective ? (
        <div className="mt-2 whitespace-pre-wrap break-words border-t border-primary/15 pt-2 text-[13px] leading-5 text-ink-2">
          {objective}
        </div>
      ) : null}
      {details.length ? <GoalDetails rows={details} /> : null}
    </div>
  )
}

function GoalTokenProgress({
  progress,
}: {
  progress: { used: number; budget: number; percent: number }
}) {
  return (
    <div className="flex w-[min(34vw,240px)] shrink-0 items-center gap-2">
      <div
        className="h-1.5 min-w-20 flex-1 overflow-hidden rounded-full bg-bg/80 shadow-inner"
        aria-label={`${numberFormatter.format(progress.used)} of ${numberFormatter.format(progress.budget)} goal tokens used`}
      >
        <div
          className="h-full rounded-full bg-primary transition-[width] duration-150"
          style={{ width: `${progress.percent}%` }}
        />
      </div>
      <span className="w-[118px] shrink-0 text-right text-[12px] tabular-nums text-ink-3">
        {numberFormatter.format(progress.used)} / {numberFormatter.format(progress.budget)}
      </span>
    </div>
  )
}

function GoalDetails({ rows }: { rows: GoalDetail[] }) {
  return (
    <div className="pointer-events-none invisible absolute right-0 bottom-full z-tooltip mb-2 w-[280px] max-w-[calc(100vw-2rem)] translate-y-1 rounded-[8px] bg-bg px-3 py-2.5 text-[12px] leading-4 opacity-0 shadow-raised ring-1 ring-border/70 transition-[opacity,transform,visibility] duration-150 group-hover/goal:pointer-events-auto group-hover/goal:visible group-hover/goal:translate-y-0 group-hover/goal:opacity-100">
      <dl className="space-y-1.5">
        {rows.map((row) => (
          <div key={row.label} className="grid grid-cols-[92px_minmax(0,1fr)] gap-3">
            <dt className="text-ink-3">{row.label}</dt>
            <dd className={`min-w-0 break-words text-ink ${row.numeric ? 'tabular-nums' : ''}`}>{row.value}</dd>
          </div>
        ))}
      </dl>
    </div>
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

function goalDotClass(status?: string, running?: boolean): string {
  if (status === 'blocked' || status === 'budgetLimited' || status === 'usageLimited') return 'bg-danger'
  if (status === 'complete') return 'bg-ink-3/60'
  if (running || status === 'active' || status === 'requested') return 'bg-primary'
  return 'bg-ink-3/60'
}

function goalTokenProgress(goal?: GoalEvent): { used: number; budget: number; percent: number } | undefined {
  if (goal?.token_budget == null) return undefined
  const used = Math.max(0, goal.tokens_used ?? 0)
  const budget = Math.max(0, goal.token_budget)
  if (budget === 0) return { used, budget, percent: used > 0 ? 100 : 0 }
  return { used, budget, percent: Math.min(100, (used / budget) * 100) }
}

function goalDetails(goal?: GoalEvent): GoalDetail[] {
  if (!goal) return []
  const rows: GoalDetail[] = []
  addDetail(rows, 'Goal tokens', numericLabel(goal.tokens_used), true)
  if (goal.token_budget != null) {
    addDetail(rows, 'Token budget', numericLabel(goal.token_budget), true)
    addDetail(rows, 'Remaining', numericLabel(goal.remaining_tokens), true)
  }
  return rows
}

function addDetail(rows: GoalDetail[], label: string, value?: string, numeric?: boolean) {
  if (!value) return
  rows.push({ label, value, numeric })
}

function numericLabel(value?: number): string {
  return typeof value === 'number' ? numberFormatter.format(value) : ''
}
