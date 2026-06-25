import type { GoalEvent } from '@/lib/api/types'

const numberFormatter = new Intl.NumberFormat()

export function GoalStatusBar({ goal, starting }: { goal?: GoalEvent; starting?: boolean }) {
  if (!goal && !starting) return null
  const objective = goal?.objective?.trim()
  const label = goalStatusLabel(goal?.status, starting)
  const budget = goalBudgetLabel(goal)
  return (
    <div className="mb-2 flex min-h-9 items-center gap-3 rounded-[8px] bg-surface px-3 py-2 text-[13px] shadow-sm ring-1 ring-border">
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 font-medium text-ink">{label}</span>
          {objective ? <span className="min-w-0 truncate text-ink-2">{objective}</span> : null}
        </div>
      </div>
      {budget ? <span className="shrink-0 tabular-nums text-ink-3">{budget}</span> : null}
    </div>
  )
}

function goalStatusLabel(status?: string, starting?: boolean): string {
  if (starting && !status) return 'Starting goal'
  switch (status) {
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

function goalBudgetLabel(goal?: GoalEvent): string {
  if (goal?.token_budget == null) return ''
  const used = goal.tokens_used ?? 0
  const remaining = goal.remaining_tokens
  const base = `${numberFormatter.format(used)} / ${numberFormatter.format(goal.token_budget)} tokens`
  return typeof remaining === 'number' ? `${base} - ${numberFormatter.format(remaining)} left` : base
}
