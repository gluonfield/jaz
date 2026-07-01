import type { GoalEvent } from '@/lib/api/types'
import { formatUsd } from '@/lib/usageCost'

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
  if (!goal) return null

  const objective = goal?.objective?.trim()
  const label = goalStatusLabel(goal?.status)
  const tokenProgress = goalTokenProgress(goal)
  const details = goalDetails(goal)

  return (
    <div className="group/goal relative mb-2 flex min-h-9 items-center gap-3 rounded-[8px] bg-primary-soft/70 px-3 py-2 text-[13px] shadow-sm ring-1 ring-primary/20">
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
      return 'Goal active'
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
  addDetail(rows, 'Token budget', goalTokenBudgetLabel(goal), true)
  addDetail(rows, 'Remaining', numericLabel(goal.remaining_tokens), true)
  addDetail(rows, 'Elapsed', elapsedLabel(goal.time_used_seconds), true)
  addDetail(rows, 'Cost', goalCostLabel(goal), true)
  addDetail(rows, 'Progress', firstText(goal.blocked_reason, goal.progress_message, goal.evaluator_reason))
  addDetail(rows, 'Operation', goal.active_operation)
  return rows
}

function addDetail(rows: GoalDetail[], label: string, value?: string, numeric?: boolean) {
  if (!value) return
  rows.push({ label, value, numeric })
}

function goalCostLabel(goal: GoalEvent): string {
  if (goal.cost_budget_usd != null) {
    const used = goal.cost_used_usd ?? 0
    return `${formatUsd(used)} / ${formatUsd(goal.cost_budget_usd)}${goal.cost_estimated ? ' est.' : ''}`
  }
  if (goal.cost_used_usd != null) return `${formatUsd(goal.cost_used_usd)}${goal.cost_estimated ? ' est.' : ''}`
  return ''
}

function goalTokenBudgetLabel(goal: GoalEvent): string {
  if (goal.token_budget != null) return numberFormatter.format(goal.token_budget)
  return goal.tokens_used != null ? 'Not set' : ''
}

function numericLabel(value?: number): string {
  return typeof value === 'number' ? numberFormatter.format(value) : ''
}

function firstText(...values: Array<string | undefined>): string {
  for (const value of values) {
    const text = value?.trim()
    if (text) return text
  }
  return ''
}

function elapsedLabel(seconds?: number): string {
  if (!seconds || seconds <= 0) return ''
  if (seconds < 60) return `${numberFormatter.format(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  if (minutes < 60) return remainder ? `${minutes}m ${remainder}s` : `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const minuteRemainder = minutes % 60
  return minuteRemainder ? `${hours}h ${minuteRemainder}m` : `${hours}h`
}
