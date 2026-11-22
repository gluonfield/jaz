import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'
import { get } from './client'

export interface CloudUsageSummary {
  month_start: string
  org_spent_usd: number
  org_monthly_cap_usd?: number | null
  user_spent_usd: number
  user_monthly_cap_usd?: number | null
}

export interface SpendStatus {
  level: 'warning' | 'exceeded'
  scope: 'user' | 'org'
  spentUsd: number
  capUsd: number
  percent: number
  message: string
}

const SPEND_WARNING_THRESHOLD = 0.8

function finiteNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function optionalPositiveCap(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

export async function fetchUsageSummary(signal?: AbortSignal): Promise<CloudUsageSummary> {
  const raw = await get<Partial<CloudUsageSummary>>('/v1/usage/summary', { signal })
  return {
    month_start: typeof raw?.month_start === 'string' ? raw.month_start : '',
    org_spent_usd: finiteNumber(raw?.org_spent_usd),
    org_monthly_cap_usd: optionalPositiveCap(raw?.org_monthly_cap_usd),
    user_spent_usd: finiteNumber(raw?.user_spent_usd),
    user_monthly_cap_usd: optionalPositiveCap(raw?.user_monthly_cap_usd),
  }
}

export const usageSummaryQuery = queryOptions({
  queryKey: keys.cloudUsageSummary,
  queryFn: ({ signal }) => fetchUsageSummary(signal),
  staleTime: 60_000,
  retry: false,
})

export function formatUsageMonth(monthStart: string): string {
  if (!monthStart) return ''
  const parsed = new Date(monthStart)
  if (Number.isNaN(parsed.getTime())) return monthStart
  return parsed.toLocaleDateString(undefined, { month: 'long', year: 'numeric', timeZone: 'UTC' })
}

export function evaluateSpendStatus(summary: CloudUsageSummary | null | undefined): SpendStatus | null {
  if (!summary) return null

  const userCap = optionalPositiveCap(summary.user_monthly_cap_usd) ?? 0
  const userSpend = finiteNumber(summary.user_spent_usd)
  const orgCap = optionalPositiveCap(summary.org_monthly_cap_usd) ?? 0
  const orgSpend = finiteNumber(summary.org_spent_usd)

  const userRatio = userCap > 0 ? userSpend / userCap : 0
  const orgRatio = orgCap > 0 ? orgSpend / orgCap : 0
  const useUser = userCap > 0 && (userRatio >= orgRatio || orgCap === 0)
  const capUsd = useUser ? userCap : orgCap
  if (capUsd <= 0) return null

  const spentUsd = useUser ? userSpend : orgSpend
  const scope: 'user' | 'org' = useUser ? 'user' : 'org'
  const ratio = spentUsd / capUsd
  const percent = Math.round(ratio * 100)

  if (spentUsd >= capUsd) {
    const label = scope === 'user' ? 'Your monthly spend cap' : 'Organization monthly spend cap'
    return {
      level: 'exceeded',
      scope,
      spentUsd,
      capUsd,
      percent: Math.max(100, percent),
      message: `${label} reached ($${spentUsd.toFixed(2)} / $${capUsd.toFixed(2)}). Contact an org admin to raise it.`,
    }
  }

  if (ratio >= SPEND_WARNING_THRESHOLD) {
    const label = scope === 'user' ? 'Monthly spend' : 'Organization spend'
    return {
      level: 'warning',
      scope,
      spentUsd,
      capUsd,
      percent,
      message: `${label} at ${percent}% ($${spentUsd.toFixed(2)} of $${capUsd.toFixed(2)} cap).`,
    }
  }

  return null
}

export function isSpendCapError(error: unknown): boolean {
  if (error instanceof ApiError && error.status === 402) return true
  if (error instanceof Error && /\b402\b|spend cap/i.test(error.message)) return true
  return false
}
