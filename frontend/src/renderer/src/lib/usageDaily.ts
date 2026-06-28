import type { DailyUsage, UsageTotals } from './api/types'

export type UsageCategoryTotals = {
  category: string
  usage: UsageTotals
}

export type UsageCell = {
  date: string
  day: DailyUsage | null
  week: number
  inRange: boolean
}

export type UsageMonthLabel = {
  label: string
  week: number
}

export type UsageModelTotals = {
  agent?: string
  model_provider?: string
  model?: string
  usage: UsageTotals
}

export const USAGE_CHART_DAYS = 182

export function visibleUsageDays(days: DailyUsage[], limit = USAGE_CHART_DAYS): DailyUsage[] {
  return days.slice(-limit)
}

export function usageCells(days: DailyUsage[]): UsageCell[] {
  if (days.length === 0) return []
  const byDate = new Map(days.map((day) => [day.date, day]))
  const first = parseLocalDate(days[0].date)
  const last = parseLocalDate(days[days.length - 1].date)
  const start = addDays(first, -first.getDay())
  const end = addDays(last, 6 - last.getDay())
  const cells: UsageCell[] = []
  for (let date = start, index = 0; date <= end; date = addDays(date, 1), index++) {
    const key = dateKey(date)
    cells.push({
      date: key,
      day: byDate.get(key) ?? null,
      week: Math.floor(index / 7),
      inRange: date >= first && date <= last,
    })
  }
  return cells
}

export function usageWeekCount(cells: UsageCell[]): number {
  return cells.length ? Math.max(...cells.map((cell) => cell.week)) + 1 : 0
}

export function usageMonthLabels(cells: UsageCell[]): UsageMonthLabel[] {
  const labels: UsageMonthLabel[] = []
  let lastWeek = -4
  for (const cell of cells) {
    const date = parseLocalDate(cell.date)
    if (date.getDate() <= 7 && cell.week - lastWeek >= 4) {
      labels.push({
        label: date.toLocaleDateString(undefined, { month: 'short' }),
        week: cell.week,
      })
      lastWeek = cell.week
    }
  }
  return labels
}

export function sumUsage(days: DailyUsage[]): UsageTotals {
  const total = days.reduce<UsageTotals>((total, day) => addUsageTotals(total, day.usage), {})
  total.input_output_tokens = totalUsageTokens(total)
  return total
}

export function sumModelUsage(days: DailyUsage[]): UsageModelTotals[] {
  return sumUsageGroups(
    days.flatMap((day) => day.models ?? []),
    modelUsageKey,
    (model) => ({
      agent: model.agent,
      model_provider: model.model_provider,
      model: model.model,
      usage: {},
    }),
    (left, right) => modelUsageKey(left).localeCompare(modelUsageKey(right)),
  )
}

export function sumCategoryUsage(days: DailyUsage[]): UsageCategoryTotals[] {
  return sumUsageGroups(
    days.flatMap((day) => day.categories ?? []),
    (category) => category.category,
    (category) => ({ category: category.category, usage: {} }),
    (left, right) => left.category.localeCompare(right.category),
  )
}

// sumUsageGroups groups rows by key, accumulates their usage into a fresh
// aggregate per group, and ranks the result by total tokens (ties broken by
// the caller). The model and category breakdowns are the same shape over
// different identities, so they share it.
function sumUsageGroups<R extends { usage: UsageTotals }, T extends { usage: UsageTotals }>(
  rows: R[],
  keyOf: (row: R) => string,
  blank: (row: R) => T,
  tiebreak: (left: T, right: T) => number,
): T[] {
  const groups = new Map<string, T>()
  for (const row of rows) {
    const key = keyOf(row)
    let group = groups.get(key)
    if (!group) {
      group = blank(row)
      groups.set(key, group)
    }
    addUsageTotals(group.usage, row.usage)
  }
  const out = [...groups.values()]
  for (const group of out) {
    group.usage.input_output_tokens = totalUsageTokens(group.usage)
  }
  return out.sort((left, right) => {
    const diff = totalUsageTokens(right.usage) - totalUsageTokens(left.usage)
    return diff !== 0 ? diff : tiebreak(left, right)
  })
}

export function peakDay(days: DailyUsage[]): DailyUsage | null {
  return days.reduce<DailyUsage | null>((peak, day) => {
    if (totalUsageTokens(day.usage) === 0) return peak
    if (!peak || totalUsageTokens(day.usage) > totalUsageTokens(peak.usage)) return day
    return peak
  }, null)
}

export function inputTokens(usage: UsageTotals): number {
  return Math.max(
    0,
    (usage.input_tokens ?? 0) - (usage.cached_input_tokens ?? 0) - (usage.cached_write_tokens ?? 0),
  )
}

export function totalUsageTokens(usage: UsageTotals): number {
  return (
    inputTokens(usage) +
    (usage.cached_input_tokens ?? 0) +
    (usage.cached_write_tokens ?? 0) +
    (usage.output_tokens ?? 0)
  )
}

export function usageLevel(total: number, maxTotal: number): number {
  if (total <= 0) return 0
  const ratio = total / maxTotal
  if (ratio < 0.18) return 1
  if (ratio < 0.42) return 2
  if (ratio < 0.72) return 3
  return 4
}

export function formatUsageDate(date: string): string {
  return parseLocalDate(date).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

function addUsageTotals(total: UsageTotals, usage: UsageTotals): UsageTotals {
  total.input_tokens = (total.input_tokens ?? 0) + (usage.input_tokens ?? 0)
  total.cached_input_tokens = (total.cached_input_tokens ?? 0) + (usage.cached_input_tokens ?? 0)
  total.cached_write_tokens = (total.cached_write_tokens ?? 0) + (usage.cached_write_tokens ?? 0)
  total.output_tokens = (total.output_tokens ?? 0) + (usage.output_tokens ?? 0)
  total.reasoning_output_tokens =
    (total.reasoning_output_tokens ?? 0) + (usage.reasoning_output_tokens ?? 0)
  return total
}

function modelUsageKey(model: UsageModelTotals): string {
  return [model.agent ?? '', model.model_provider ?? '', model.model ?? ''].join('\u0000')
}

function parseLocalDate(date: string): Date {
  return new Date(`${date}T00:00:00`)
}

function addDays(date: Date, days: number): Date {
  const next = new Date(date)
  next.setDate(next.getDate() + days)
  return next
}

function dateKey(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
