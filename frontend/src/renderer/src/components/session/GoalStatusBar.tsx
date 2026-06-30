import type { GoalEvent } from '@/lib/api/types'
import { formatUsd } from '@/lib/usageCost'

const numberFormatter = new Intl.NumberFormat()
const dateTimeFormatter = new Intl.DateTimeFormat(undefined, {
  dateStyle: 'medium',
  timeStyle: 'short',
})

type GoalMetadataRow = {
  label: string
  value: string
  compact?: boolean
}

export function GoalStatusBar({
  goal,
  starting,
  running,
}: {
  goal?: GoalEvent
  starting?: boolean
  running?: boolean
}) {
  if (!goal && !starting) return null
  const objective = goal?.objective?.trim()
  const label = goalStatusLabel(goal?.status, starting, running)
  const budget = goalBudgetLabel(goal)
  const details = goalDetailLabel(goal)
  const metadata = goalMetadataRows(goal)
  return (
    <div className="group/goal relative mb-2 flex min-h-9 items-center gap-3 rounded-[8px] bg-primary-soft/70 px-3 py-2 text-[13px] shadow-sm ring-1 ring-primary/20">
      <span className={`size-1.5 shrink-0 rounded-full ${goalDotClass(goal?.status, running || starting)}`} />
      <div className="min-w-0 flex-1 leading-5">
        <div className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 font-medium text-primary-strong">{label}</span>
          {objective ? <span className="min-w-0 truncate text-ink-2">{objective}</span> : null}
        </div>
        {details ? <div className="truncate text-[12px] text-ink-3">{details}</div> : null}
      </div>
      {budget ? <span className="shrink-0 tabular-nums text-ink-3">{budget}</span> : null}
      {metadata.length ? <GoalMetadataPanel rows={metadata} /> : null}
    </div>
  )
}

function GoalMetadataPanel({ rows }: { rows: GoalMetadataRow[] }) {
  return (
    <div className="pointer-events-none invisible absolute right-0 bottom-full z-tooltip mb-2 max-h-[min(70vh,520px)] w-[360px] max-w-[calc(100vw-2rem)] translate-y-1 overflow-y-auto rounded-[8px] bg-bg px-3 py-2.5 text-[12px] leading-4 opacity-0 shadow-raised ring-1 ring-border/70 transition-[opacity,transform,visibility] duration-150 group-hover/goal:pointer-events-auto group-hover/goal:visible group-hover/goal:translate-y-0 group-hover/goal:opacity-100">
      <div className="mb-2 flex items-center justify-between gap-3 border-b border-border/70 pb-2">
        <span className="font-medium text-ink">Goal metadata</span>
        <span className="text-[11px] text-ink-3">provider report</span>
      </div>
      <dl className="space-y-1.5">
        {rows.map((row) => (
          <div key={row.label} className="grid grid-cols-[108px_minmax(0,1fr)] gap-3">
            <dt className="text-ink-3">{row.label}</dt>
            <dd className={`min-w-0 break-words text-ink ${row.compact ? 'font-mono text-[11px] tabular-nums' : ''}`}>
              {row.value}
            </dd>
          </div>
        ))}
      </dl>
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

function goalBudgetLabel(goal?: GoalEvent): string {
  if (goal?.token_budget != null) {
    const used = goal.tokens_used ?? 0
    const remaining = goal.remaining_tokens
    const base = `${numberFormatter.format(used)} / ${numberFormatter.format(goal.token_budget)} tokens`
    return typeof remaining === 'number' ? `${base} - ${numberFormatter.format(remaining)} left` : base
  }
  if (goal?.cost_budget_usd != null) {
    const used = goal.cost_used_usd ?? 0
    const suffix = goal.cost_estimated ? ' est.' : ''
    return `${formatUsd(used)} / ${formatUsd(goal.cost_budget_usd)}${suffix}`
  }
  if (goal?.cost_used_usd != null) {
    const suffix = goal.cost_estimated ? ' est.' : ''
    return `${formatUsd(goal.cost_used_usd)} spent${suffix}`
  }
  return ''
}

function goalMetadataRows(goal?: GoalEvent): GoalMetadataRow[] {
  if (!goal) return []
  const rows: GoalMetadataRow[] = []
  addMetadataRow(rows, 'Objective', goal.objective)
  addMetadataRow(rows, 'Status', goal.status)
  addMetadataRow(rows, 'Provider', goal.provider)
  addMetadataRow(rows, 'Goal ID', goal.provider_goal_id, true)
  addMetadataRow(rows, 'Budget source', goal.budget_source)
  addMetadataRow(rows, 'Tokens used', numericLabel(goal.tokens_used), true)
  addMetadataRow(rows, 'Token budget', numericLabel(goal.token_budget), true)
  addMetadataRow(rows, 'Tokens left', numericLabel(goal.remaining_tokens), true)
  addMetadataRow(rows, 'Cost used', costLabel(goal.cost_used_usd, goal.cost_estimated), true)
  addMetadataRow(rows, 'Cost budget', costLabel(goal.cost_budget_usd), true)
  addMetadataRow(rows, 'Cost estimated', boolLabel(goal.cost_estimated))
  addMetadataRow(rows, 'Elapsed', elapsedLabel(goal.time_used_seconds), true)
  addMetadataRow(rows, 'Turns', numericLabel(goal.turn_count), true)
  addMetadataRow(rows, 'Evaluated', numericLabel(goal.evaluated_turns), true)
  addMetadataRow(rows, 'Attempts', numericLabel(goal.attempt_count), true)
  addMetadataRow(rows, 'Progress', goal.progress_message)
  addMetadataRow(rows, 'Blocked', goal.blocked_reason)
  addMetadataRow(rows, 'Evaluator', goal.evaluator_reason)
  addMetadataRow(rows, 'Review', goal.completion_review)
  addMetadataRow(rows, 'Operation', goal.active_operation)
  addMetadataRow(rows, 'Subagent', goal.active_subagent_id, true)
  addMetadataRow(rows, 'Created', dateTimeLabel(goal.created_at), true)
  addMetadataRow(rows, 'Updated', dateTimeLabel(goal.updated_at), true)
  addMetadataRow(rows, 'Completed', dateTimeLabel(goal.completed_at), true)
  addMetadataRow(rows, 'Thread', goal.thread_id, true)
  return rows
}

function addMetadataRow(rows: GoalMetadataRow[], label: string, value?: string, compact?: boolean) {
  if (!value) return
  rows.push({ label, value, compact })
}

function numericLabel(value?: number): string {
  return typeof value === 'number' ? numberFormatter.format(value) : ''
}

function costLabel(value?: number, estimated?: boolean): string {
  if (typeof value !== 'number') return ''
  return `${formatUsd(value)}${estimated ? ' est.' : ''}`
}

function boolLabel(value?: boolean): string {
  return typeof value === 'boolean' ? (value ? 'Yes' : 'No') : ''
}

function dateTimeLabel(value?: string): string {
  if (!value) return ''
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return value
  return dateTimeFormatter.format(new Date(timestamp))
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
