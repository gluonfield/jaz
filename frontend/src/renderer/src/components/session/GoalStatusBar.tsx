import type { GoalEvent, Usage } from '@/lib/api/types'
import { formatTokens } from '@/lib/format/tokens'

const numberFormatter = new Intl.NumberFormat()
const costFormatter = new Intl.NumberFormat(undefined, {
  currency: 'USD',
  maximumFractionDigits: 2,
  minimumFractionDigits: 2,
  style: 'currency',
})

export function GoalStatusBar({
  goal,
  starting,
  running,
  usage,
}: {
  goal?: GoalEvent
  starting?: boolean
  running?: boolean
  usage?: Usage
}) {
  if (!goal && !starting) return null
  const objective = goal?.objective?.trim()
  const label = goalStatusLabel(goal?.status, starting, running)
  const budget = goalBudgetLabel(goal, usage)
  const details = goalDetailLabel(goal)
  return (
    <div className="mb-2 flex min-h-9 items-center gap-3 rounded-[8px] bg-primary-soft/70 px-3 py-2 text-[13px] shadow-sm ring-1 ring-primary/20">
      <span className={`size-1.5 shrink-0 rounded-full ${goalDotClass(goal?.status, running || starting)}`} />
      <div className="min-w-0 flex-1 leading-5">
        <div className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 font-medium text-primary-strong">{label}</span>
          {objective ? <span className="min-w-0 truncate text-ink-2">{objective}</span> : null}
        </div>
        {details ? <div className="truncate text-[12px] text-ink-3">{details}</div> : null}
      </div>
      {budget ? <span className="shrink-0 tabular-nums text-ink-3">{budget}</span> : null}
    </div>
  )
}

function goalStatusLabel(status?: string, starting?: boolean, running?: boolean): string {
  if (starting && !status) return 'Starting goal'
  switch (status) {
    case 'active':
      return 'Goal active'
    case 'requested':
      return running ? 'Goal active' : 'Goal requested'
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

function goalBudgetLabel(goal?: GoalEvent, usage?: Usage): string {
  if (goal?.token_budget != null) {
    const used = goal.tokens_used ?? 0
    const remaining = goal.remaining_tokens
    const base = `${numberFormatter.format(used)} / ${numberFormatter.format(goal.token_budget)} tokens`
    return typeof remaining === 'number' ? `${base} - ${numberFormatter.format(remaining)} left` : base
  }
  if (goal?.cost_budget_usd != null) {
    const used = goal.cost_used_usd ?? 0
    const suffix = goal.cost_estimated ? ' est.' : ''
    return `${costFormatter.format(used)} / ${costFormatter.format(goal.cost_budget_usd)}${suffix}`
  }
  if (goal?.cost_used_usd != null) {
    const suffix = goal.cost_estimated ? ' est.' : ''
    return `${costFormatter.format(goal.cost_used_usd)} spent${suffix}`
  }
  const context = usage?.context_tokens ?? 0
  const contextWindow = usage?.context_window_tokens ?? 0
  if (context > 0 && contextWindow > 0) {
    return `Context ${formatTokens(context)} / ${formatTokens(contextWindow)}`
  }
  if (context > 0) return `Context ${formatTokens(context)}`
  return ''
}

function goalDetailLabel(goal?: GoalEvent): string {
  if (!goal) return ''
  const parts = [
    firstText(goal.blocked_reason, goal.progress_message, goal.evaluator_reason),
    goalTurnLabel(goal),
    elapsedLabel(goal.time_used_seconds),
    goal.active_operation ? `Operation ${goal.active_operation}` : '',
    goal.active_subagent_id ? `Subagent ${goal.active_subagent_id}` : '',
    goal.completion_review ? `Review ${goal.completion_review}` : '',
  ].filter(Boolean)
  return parts.join(' | ')
}

function firstText(...values: Array<string | undefined>): string {
  for (const value of values) {
    const text = value?.trim()
    if (text) return text
  }
  return ''
}

function goalTurnLabel(goal: GoalEvent): string {
  const evaluated = goal.evaluated_turns ?? 0
  if (evaluated > 0) return `${numberFormatter.format(evaluated)} evaluated turns`
  const turns = goal.turn_count ?? 0
  if (turns > 0) return `${numberFormatter.format(turns)} turns`
  const attempts = goal.attempt_count ?? 0
  if (attempts > 0) return `${numberFormatter.format(attempts)} attempts`
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
