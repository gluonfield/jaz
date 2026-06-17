import type { DailyUsage, UsageTotals } from './api/types'

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
  const total = days.reduce<UsageTotals>((total, day) => {
    total.input_tokens = (total.input_tokens ?? 0) + (day.usage.input_tokens ?? 0)
    total.cached_input_tokens = (total.cached_input_tokens ?? 0) + (day.usage.cached_input_tokens ?? 0)
    total.cached_write_tokens = (total.cached_write_tokens ?? 0) + (day.usage.cached_write_tokens ?? 0)
    total.output_tokens = (total.output_tokens ?? 0) + (day.usage.output_tokens ?? 0)
    total.reasoning_output_tokens =
      (total.reasoning_output_tokens ?? 0) + (day.usage.reasoning_output_tokens ?? 0)
    return total
  }, {})
  total.input_output_tokens = inputOutputTokens(total)
  return total
}

export function peakDay(days: DailyUsage[]): DailyUsage | null {
  return days.reduce<DailyUsage | null>((peak, day) => {
    if (inputOutputTokens(day.usage) === 0) return peak
    if (!peak || inputOutputTokens(day.usage) > inputOutputTokens(peak.usage)) return day
    return peak
  }, null)
}

export function inputOutputTokens(usage: UsageTotals): number {
  return usage.input_output_tokens ?? (usage.input_tokens ?? 0) + (usage.output_tokens ?? 0)
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
