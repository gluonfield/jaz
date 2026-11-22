import { useQuery } from '@tanstack/react-query'
import { ChartNoAxesColumn } from 'lucide-react'
import { type MouseEvent, useMemo, useState } from 'react'
import { UsageWindowFilter } from '@/components/settings/UsageWindowFilter'
import type { UsageWindow } from '@/lib/usageWindow'
import { CategoryBreakdown } from '@/components/settings/UsageCategoryBreakdown'
import { ModelBreakdown, UsageShareCharts } from '@/components/settings/UsageModelBreakdown'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Skeleton } from '@/components/ui/Skeleton'
import { agentSettingsQuery } from '@/lib/api/settings'
import { evaluateSpendStatus, formatUsageMonth, usageSummaryQuery } from '@/lib/api/cloudUsage'
import { dailyUsageQuery } from '@/lib/api/sessions'
import type { AgentSettings, DailyUsage } from '@/lib/api/types'
import { useIsCloud } from '@/lib/connection'
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
  sumCategoryUsage,
  type UsageCell,
  usageCells,
  usageLevel,
  usageMonthLabels,
  usageWeekCount,
} from '@/lib/usageDaily'

type TooltipState = {
  day: DailyUsage
  x: number
  y: number
}

const usageSquarePx = 12
const usageGapPx = 3

export function UsageSettings() {
  const isCloud = useIsCloud()
  const [window, setWindow] = useState<UsageWindow>({ days: 30 })
  const usage = useQuery({ ...dailyUsageQuery(window), enabled: !isCloud })
  const cloudSummary = useQuery({ ...usageSummaryQuery, enabled: isCloud })
  const agentSettings = useQuery({ ...agentSettingsQuery, enabled: !isCloud })
  const openRouter = useQuery({ ...openRouterModelsQuery, enabled: !isCloud })
  const pricing = useMemo(() => buildPricingIndex(openRouter.data?.models ?? []), [openRouter.data])
  const spendStatus = evaluateSpendStatus(cloudSummary.data)
  const monthLabel = cloudSummary.data ? formatUsageMonth(cloudSummary.data.month_start) : ''

  return (
    <section className="py-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">Usage</p>
          <p className="mt-0.5 text-[13px] text-ink-2">
            {isCloud
              ? 'Monthly cloud spend and active organization caps.'
              : 'Daily input and output tokens across all agent sessions, excluding cache reads.'}
          </p>
        </div>
      </div>

      {isCloud ? (
        cloudSummary.isPending ? (
          <SettingsCard className="mt-4 p-3.5">
            <Skeleton className="h-14" />
          </SettingsCard>
        ) : cloudSummary.isError ? (
          <p className="mt-4 py-2 text-[13px] text-danger">{cloudSummary.error.message}</p>
        ) : cloudSummary.data ? (
          <SettingsCard className="mt-4 p-3.5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <p className="text-[12px] font-medium text-ink">
                  Monthly cloud spend{monthLabel ? ` · ${monthLabel}` : ''}
                </p>
                {spendStatus ? (
                  <p className={`mt-0.5 text-[12px] ${spendStatus.level === 'exceeded' ? 'text-danger' : 'text-accent-strong'}`}>
                    {spendStatus.message}
                  </p>
                ) : null}
              </div>
              <div className="flex items-center gap-4 font-mono text-[13px] tabular-nums">
                <span className="text-ink-2">
                  You <span className="text-ink">${cloudSummary.data.user_spent_usd.toFixed(2)}</span>
                  {cloudSummary.data.user_monthly_cap_usd ? ` / $${cloudSummary.data.user_monthly_cap_usd.toFixed(2)}` : ''}
                </span>
                <span className="text-ink-2">
                  Org <span className="text-ink">${cloudSummary.data.org_spent_usd.toFixed(2)}</span>
                  {cloudSummary.data.org_monthly_cap_usd ? ` / $${cloudSummary.data.org_monthly_cap_usd.toFixed(2)}` : ''}
                </span>
              </div>
            </div>
          </SettingsCard>
        ) : null
      ) : (
        <>
          <UsageWindowFilter window={window} onChange={setWindow} />

          {usage.isPending ? (
            <UsageSkeleton />
          ) : usage.isError ? (
            <p className="mt-4 py-2 text-[13px] text-danger">{usage.error.message}</p>
          ) : (
            <UsagePanel
              key={JSON.stringify(window)}
              days={usage.data}
              pricing={pricing}
              agentSettings={agentSettings.data}
            />
          )}
        </>
      )}
    </section>
  )
}

function UsageSkeleton() {
  return (
    <SettingsCard className="mt-4 p-3.5">
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
  agentSettings,
}: {
  days: DailyUsage[]
  pricing: Map<string, ModelPricing>
  agentSettings?: AgentSettings
}) {
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const cells = useMemo(() => usageCells(days), [days])
  const monthLabels = useMemo(() => usageMonthLabels(cells), [cells])
  const weekCount = useMemo(() => usageWeekCount(cells), [cells])
  const total = sumUsage(days)
  const models = useMemo(() => sumModelUsage(days), [days])
  const categories = useMemo(() => sumCategoryUsage(days), [days])
  const peak = peakDay(days)
  const activeDays = days.filter((day) => totalUsageTokens(day.usage) > 0).length
  const maxTotal = Math.max(1, ...days.map((day) => totalUsageTokens(day.usage)))
  const hasUsage = days.some((day) => totalUsageTokens(day.usage) > 0)
  const inputAndCacheTokens = total.input_tokens ?? 0
  const cacheRead = total.cached_input_tokens ?? 0
  const cacheWrite = total.cached_write_tokens ?? 0
  const reasoning = total.reasoning_output_tokens ?? 0
  const cacheHitLabel = inputAndCacheTokens > 0 ? `${Math.round((cacheRead / inputAndCacheTokens) * 100)}%` : '—'
  const { rows: pricedModels, summary: costSummary } = useMemo(
    () => priceModels(models, pricing, agentSettings),
    [agentSettings, models, pricing],
  )
  const periodLabel = days.length === 0 ? '' : days.length === 1
    ? formatUsageDate(days[0].date)
    : `${formatUsageDate(days[0].date)} – ${formatUsageDate(days[days.length - 1].date)}`
  const costLabel = costSummary.priced > 0 ? formatUsd(costSummary.total) : '—'
  const dailyCostLabels = useMemo(() => {
    const labels = new Map<string, string>()
    for (const day of days) {
      labels.set(day.date, dailyCostLabel(day, pricing, agentSettings))
    }
    return labels
  }, [agentSettings, days, pricing])

  return (
    <SettingsCard className="mt-4 p-3.5">
      <p className="mb-3 text-[12px] text-ink-2">{periodLabel}</p>
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-control bg-border/70 md:grid-cols-4">
        <UsageStat label="Total tokens" value={formatTokens(totalUsageTokens(total))} detail="input + output" />
        <UsageStat label="Daily average" value={formatTokens(Math.round(totalUsageTokens(total) / Math.max(1, days.length)))} detail="tokens per day" />
        <UsageStat label="Est. cost" value={costLabel} detail="list prices" />
        <UsageStat label="Cache hit rate" value={cacheHitLabel} detail="of input + cache" />
      </div>

      <div className="mt-4 min-w-0 rounded-control bg-bg/45 px-3 py-2.5">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2 text-[12px] font-medium text-ink">
            <ChartNoAxesColumn size={14} className="text-ink-3" />
            <span>Daily usage</span>
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

        <div className="overflow-x-auto pb-1">
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
          {!hasUsage ? <span>No token usage in this period.</span> : null}
          <span>{activeDays} active day{activeDays === 1 ? '' : 's'} in {days.length} day{days.length === 1 ? '' : 's'}</span>
          {peak ? <span>Peak {formatUsageDate(peak.date)}: {formatTokens(totalUsageTokens(peak.usage))}</span> : null}
          {cacheRead > 0 ? <span>Cache read {formatTokens(cacheRead)}</span> : null}
          {cacheWrite > 0 ? <span>Cache write {formatTokens(cacheWrite)}</span> : null}
          {reasoning > 0 ? <span>Reasoning {formatTokens(reasoning)}</span> : null}
        </div>
      </div>

      <CategoryBreakdown categories={categories} />

      <UsageShareCharts rows={pricedModels} />

      <ModelBreakdown
        rows={pricedModels}
        cost={costSummary}
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

function dailyCostLabel(
  day: DailyUsage,
  pricing: Map<string, ModelPricing>,
  agentSettings?: AgentSettings,
): string {
  const models = day.models ?? []
  if (models.length === 0) return '—'
  const { summary } = priceModels(models, pricing, agentSettings)
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
