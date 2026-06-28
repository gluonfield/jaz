import { useQuery } from '@tanstack/react-query'
import { ChartNoAxesColumn } from 'lucide-react'
import { type MouseEvent, useMemo, useState } from 'react'
import { ModelBreakdown, UsageShareCharts } from '@/components/settings/UsageModelBreakdown'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Skeleton } from '@/components/ui/Skeleton'
import { dailyUsageQuery } from '@/lib/api/sessions'
import type { DailyUsage } from '@/lib/api/types'
import { formatTokens } from '@/lib/format/tokens'
import { type ModelPricing, openRouterModelsQuery } from '@/lib/models'
import { buildPricingIndex, formatUsd, priceModels } from '@/lib/usageCost'
import {
  formatUsageDate,
  inputTokens,
  peakDay,
  sumModelUsage,
  sumUsage,
  totalUsageTokens,
  USAGE_CHART_DAYS,
  type UsageCell,
  usageCells,
  usageLevel,
  usageMonthLabels,
  usageWeekCount,
  visibleUsageDays,
} from '@/lib/usageDaily'

type TooltipState = {
  day: DailyUsage
  x: number
  y: number
}

const usageSquarePx = 12
const usageGapPx = 3

export function UsageSettings() {
  const usage = useQuery(dailyUsageQuery(365))
  const openRouter = useQuery(openRouterModelsQuery)
  const pricing = useMemo(() => buildPricingIndex(openRouter.data ?? []), [openRouter.data])

  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">Usage</p>
          <p className="mt-0.5 text-[13px] text-ink-2">Daily token totals across all agent sessions.</p>
        </div>
      </div>

      {usage.isPending ? (
        <UsageSkeleton />
      ) : usage.isError ? (
        <p className="mt-4 py-2 text-[13px] text-danger">{usage.error.message}</p>
      ) : (
        <UsagePanel
          days={usage.data}
          pricing={pricing}
        />
      )}
    </section>
  )
}

function UsageSkeleton() {
  return (
    <SettingsCard className="mt-4 p-4">
      <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className="h-12" />
        ))}
      </div>
      <Skeleton className="mt-4 h-[130px]" />
      <Skeleton className="mt-4 h-[150px]" />
    </SettingsCard>
  )
}

function UsagePanel({
  days,
  pricing,
}: {
  days: DailyUsage[]
  pricing: Map<string, ModelPricing>
}) {
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const chartDays = useMemo(() => visibleUsageDays(days), [days])
  const cells = useMemo(() => usageCells(chartDays), [chartDays])
  const monthLabels = useMemo(() => usageMonthLabels(cells), [cells])
  const weekCount = useMemo(() => usageWeekCount(cells), [cells])
  const last30Days = useMemo(() => days.slice(-30), [days])
  const last7 = sumUsage(days.slice(-7))
  const last30 = sumUsage(last30Days)
  const models = useMemo(() => sumModelUsage(last30Days), [last30Days])
  const peak = peakDay(chartDays)
  const activeDays = chartDays.filter((day) => totalUsageTokens(day.usage) > 0).length
  const maxTotal = Math.max(1, ...chartDays.map((day) => totalUsageTokens(day.usage)))
  const hasUsage = chartDays.some((day) => totalUsageTokens(day.usage) > 0)
  const inputAndCacheTokens = last30.input_tokens ?? 0
  const cacheRead = last30.cached_input_tokens ?? 0
  const cacheWrite = last30.cached_write_tokens ?? 0
  const reasoning = last30.reasoning_output_tokens ?? 0
  const cacheHitLabel = inputAndCacheTokens > 0 ? `${Math.round((cacheRead / inputAndCacheTokens) * 100)}%` : '—'
  const { rows: pricedModels, summary: costSummary } = useMemo(() => priceModels(models, pricing), [models, pricing])
  const costLabel = costSummary.priced > 0 ? formatUsd(costSummary.total) : '—'
  const dailyCostLabels = useMemo(() => {
    const labels = new Map<string, string>()
    for (const day of chartDays) {
      labels.set(day.date, dailyCostLabel(day, pricing))
    }
    return labels
  }, [chartDays, pricing])

  return (
    <SettingsCard className="mt-4 p-4">
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-control bg-border/70 md:grid-cols-4">
        <UsageStat label="Last 7 days" value={formatTokens(totalUsageTokens(last7))} detail="total tokens" />
        <UsageStat label="Last 30 days" value={formatTokens(totalUsageTokens(last30))} detail="total tokens" />
        <UsageStat label="Est. cost" value={costLabel} detail="30d · list prices" />
        <UsageStat label="Cache hit rate" value={cacheHitLabel} detail="of input + cache" />
      </div>

      <div className="mt-5 min-w-0 rounded-control bg-bg/45 px-3 py-3">
        <div className="mb-2 flex items-center justify-between gap-4">
          <div className="flex items-center gap-2 text-[12px] font-medium text-ink">
            <ChartNoAxesColumn size={14} className="text-ink-3" />
            <span>Last 6 months</span>
          </div>
          <div className="flex items-center gap-1.5 text-[11px] text-ink-3">
            <span>Less</span>
            {[0, 1, 2, 3, 4].map((level) => (
              <span
                key={level}
                className="size-2.5 rounded-[3px] ring-1 ring-border/60"
                style={{ background: levelColor(level) }}
              />
            ))}
            <span>More</span>
          </div>
        </div>

        <div className="pb-1">
          <div
            className="grid w-full grid-cols-[24px_auto] gap-x-2 gap-y-1"
            style={{ gridTemplateRows: `16px repeat(7, ${usageSquarePx}px)` }}
          >
            <div />
            <div
              className="grid h-4 gap-[3px]"
              style={{
                gap: `${usageGapPx}px`,
                gridTemplateColumns: `repeat(${weekCount}, ${usageSquarePx}px)`,
              }}
            >
              {monthLabels.map((label) => (
                <span
                  key={`${label.label}-${label.week}`}
                  className="text-[10px] leading-none text-ink-3"
                  style={{ gridColumn: `${label.week + 1} / span 3` }}
                >
                  {label.label}
                </span>
              ))}
            </div>

            <div
              className="grid"
              style={{
                gap: `${usageGapPx}px`,
                gridColumn: 1,
                gridRow: '2 / span 7',
                gridTemplateRows: `repeat(7, ${usageSquarePx}px)`,
              }}
            >
              {['', 'Mon', '', 'Wed', '', 'Fri', ''].map((label, index) => (
                <div
                  key={`${label}-${index}`}
                  className="text-right text-[10px] text-ink-3"
                  style={{ height: usageSquarePx, lineHeight: `${usageSquarePx}px` }}
                >
                  {label}
                </div>
              ))}
            </div>

            <div
              className="grid grid-flow-col grid-rows-7"
              style={{
                gap: `${usageGapPx}px`,
                gridColumn: 2,
                gridRow: '2 / span 7',
                gridTemplateColumns: `repeat(${weekCount}, ${usageSquarePx}px)`,
              }}
            >
              {cells.map((cell) => (
                <UsageSquare
                  key={cell.date}
                  cell={cell}
                  maxTotal={maxTotal}
                  costLabel={cell.day ? dailyCostLabels.get(cell.day.date) ?? '—' : '—'}
                  onHover={setTooltip}
                />
              ))}
            </div>
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-ink-3">
          {!hasUsage ? <span>No token usage recorded yet.</span> : null}
          <span>{activeDays} active day{activeDays === 1 ? '' : 's'} in {USAGE_CHART_DAYS} days</span>
          {peak ? <span>Peak {formatUsageDate(peak.date)}: {formatTokens(totalUsageTokens(peak.usage))}</span> : null}
          {cacheRead > 0 ? <span>Cache read {formatTokens(cacheRead)}</span> : null}
          {cacheWrite > 0 ? <span>Cache write {formatTokens(cacheWrite)}</span> : null}
          {reasoning > 0 ? <span>Reasoning {formatTokens(reasoning)}</span> : null}
        </div>
      </div>

      <UsageShareCharts rows={pricedModels} />

      <ModelBreakdown
        rows={pricedModels}
        unpriced={costSummary.unpriced}
      />

      {tooltip ? <UsageTooltip state={tooltip} costLabel={dailyCostLabels.get(tooltip.day.date) ?? '—'} /> : null}
    </SettingsCard>
  )
}

function UsageStat({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="min-w-0 bg-bg px-3 py-2">
      <div className="truncate font-mono text-[15px] leading-none text-ink tabular-nums">{value}</div>
      <div className="mt-1 truncate text-[11px] font-medium text-ink-2">{label}</div>
      <div className="mt-0.5 truncate text-[10px] text-ink-3">{detail}</div>
    </div>
  )
}

function UsageSquare({
  cell,
  maxTotal,
  costLabel,
  onHover,
}: {
  cell: UsageCell
  maxTotal: number
  costLabel: string
  onHover: (state: TooltipState | null) => void
}) {
  const day = cell.day
  const dayTotal = day ? totalUsageTokens(day.usage) : 0
  const level = cell.inRange ? usageLevel(dayTotal, maxTotal) : 0

  if (!day) {
    return <span style={{ width: usageSquarePx, height: usageSquarePx }} />
  }
  const activeDay = day

  function show(event: MouseEvent<HTMLButtonElement>) {
    onHover({ day: activeDay, x: event.clientX, y: event.clientY })
  }

  return (
    <button
      type="button"
      aria-label={usageTooltipText(activeDay, costLabel).replace(/\n/g, ', ')}
      onMouseEnter={show}
      onMouseMove={show}
      onMouseLeave={() => onHover(null)}
      className="cursor-default rounded-[3px] ring-1 ring-border/60 transition-[scale,box-shadow] duration-150 hover:scale-125 hover:ring-ink/25"
      style={{ width: usageSquarePx, height: usageSquarePx, background: levelColor(level) }}
    />
  )
}

function UsageTooltip({ state, costLabel }: { state: TooltipState; costLabel: string }) {
  const left = Math.max(8, Math.min(window.innerWidth - 238, state.x + 14))
  const top = Math.max(8, state.y - 112)
  const cacheRead = state.day.usage.cached_input_tokens ?? 0
  const cacheWrite = state.day.usage.cached_write_tokens ?? 0
  const reasoning = state.day.usage.reasoning_output_tokens ?? 0

  return (
    <div
      className="pointer-events-none fixed z-tooltip w-[220px] rounded-control bg-bg px-3 py-2 text-[11px] text-ink shadow-raised ring-1 ring-border/70"
      style={{ left, top }}
    >
      <div className="font-medium">{formatUsageDate(state.day.date)}</div>
      <div className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
        <span className="text-ink-3">Input</span>
        <span className="text-right font-mono tabular-nums">{formatTokens(inputTokens(state.day.usage))}</span>
        <span className="text-ink-3">Output</span>
        <span className="text-right font-mono tabular-nums">{formatTokens(state.day.usage.output_tokens ?? 0)}</span>
        <span className="text-ink-3">Total</span>
        <span className="text-right font-mono tabular-nums">{formatTokens(totalUsageTokens(state.day.usage))}</span>
        <span className="text-ink-3">Est. cost</span>
        <span className="text-right font-mono tabular-nums">{costLabel}</span>
        {cacheRead > 0 ? (
          <>
            <span className="text-ink-3">Cache read</span>
            <span className="text-right font-mono tabular-nums">{formatTokens(cacheRead)}</span>
          </>
        ) : null}
        {cacheWrite > 0 ? (
          <>
            <span className="text-ink-3">Cache write</span>
            <span className="text-right font-mono tabular-nums">{formatTokens(cacheWrite)}</span>
          </>
        ) : null}
        {reasoning > 0 ? (
          <>
            <span className="text-ink-3">Reasoning</span>
            <span className="text-right font-mono tabular-nums">{formatTokens(reasoning)}</span>
          </>
        ) : null}
      </div>
    </div>
  )
}

function levelColor(level: number): string {
  switch (level) {
    case 1:
      return 'color-mix(in oklab, var(--color-primary) 18%, var(--color-bg))'
    case 2:
      return 'color-mix(in oklab, var(--color-primary) 36%, var(--color-bg))'
    case 3:
      return 'color-mix(in oklab, var(--color-primary) 62%, var(--color-bg))'
    case 4:
      return 'var(--color-primary)'
    default:
      return 'var(--color-bg)'
  }
}

function dailyCostLabel(day: DailyUsage, pricing: Map<string, ModelPricing>): string {
  const models = day.models ?? []
  if (models.length === 0) return '—'
  const { summary } = priceModels(models, pricing)
  return summary.priced > 0 ? formatUsd(summary.total) : '—'
}

function usageTooltipText(day: DailyUsage, costLabel: string): string {
  return [
    formatUsageDate(day.date),
    `Input ${formatTokens(inputTokens(day.usage))}`,
    `Output ${formatTokens(day.usage.output_tokens ?? 0)}`,
    `Total ${formatTokens(totalUsageTokens(day.usage))}`,
    `Est. cost ${costLabel}`,
    `Cache read ${formatTokens(day.usage.cached_input_tokens ?? 0)}`,
    `Cache write ${formatTokens(day.usage.cached_write_tokens ?? 0)}`,
  ].join('\n')
}
