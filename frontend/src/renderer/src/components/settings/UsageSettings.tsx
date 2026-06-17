import { useQuery } from '@tanstack/react-query'
import { ChartNoAxesColumn } from 'lucide-react'
import { type MouseEvent, useMemo, useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Skeleton } from '@/components/ui/Skeleton'
import { dailyUsageQuery, modelUsageQuery } from '@/lib/api/sessions'
import type { DailyUsage, ModelUsage } from '@/lib/api/types'
import { formatTokens } from '@/lib/format/tokens'
import {
  formatUsageDate,
  inputOutputTokens,
  peakDay,
  sumUsage,
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
  const models = useQuery(modelUsageQuery(30))

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
          models={models.data ?? []}
          modelsError={models.error}
          modelsPending={models.isPending}
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
  models,
  modelsError,
  modelsPending,
}: {
  days: DailyUsage[]
  models: ModelUsage[]
  modelsError: Error | null
  modelsPending: boolean
}) {
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const chartDays = useMemo(() => visibleUsageDays(days), [days])
  const cells = useMemo(() => usageCells(chartDays), [chartDays])
  const monthLabels = useMemo(() => usageMonthLabels(cells), [cells])
  const weekCount = useMemo(() => usageWeekCount(cells), [cells])
  const last7 = sumUsage(days.slice(-7))
  const last30 = sumUsage(days.slice(-30))
  const peak = peakDay(chartDays)
  const activeDays = chartDays.filter((day) => inputOutputTokens(day.usage) > 0).length
  const maxTotal = Math.max(1, ...chartDays.map((day) => inputOutputTokens(day.usage)))
  const hasUsage = chartDays.some((day) => inputOutputTokens(day.usage) > 0)
  const cacheRead = last30.cached_input_tokens ?? 0
  const cacheWrite = last30.cached_write_tokens ?? 0
  const reasoning = last30.reasoning_output_tokens ?? 0

  return (
    <SettingsCard className="mt-4 p-4">
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-control bg-border/70 md:grid-cols-4">
        <UsageStat label="Last 7 days" value={formatTokens(inputOutputTokens(last7))} detail="input + output" />
        <UsageStat label="Last 30 days" value={formatTokens(inputOutputTokens(last30))} detail="input + output" />
        <UsageStat label="Input" value={formatTokens(last30.input_tokens ?? 0)} detail="last 30 days" />
        <UsageStat label="Output" value={formatTokens(last30.output_tokens ?? 0)} detail="last 30 days" />
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
                  onHover={setTooltip}
                />
              ))}
            </div>
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-ink-3">
          {!hasUsage ? <span>No token usage recorded yet.</span> : null}
          <span>{activeDays} active day{activeDays === 1 ? '' : 's'} in {USAGE_CHART_DAYS} days</span>
          {peak ? <span>Peak {formatUsageDate(peak.date)}: {formatTokens(inputOutputTokens(peak.usage))}</span> : null}
          {cacheRead > 0 ? <span>Cache read {formatTokens(cacheRead)}</span> : null}
          {cacheWrite > 0 ? <span>Cache write {formatTokens(cacheWrite)}</span> : null}
          {reasoning > 0 ? <span>Reasoning {formatTokens(reasoning)}</span> : null}
        </div>
      </div>

      <ModelBreakdown models={models} error={modelsError} pending={modelsPending} />

      {tooltip ? <UsageTooltip state={tooltip} /> : null}
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

function ModelBreakdown({
  models,
  error,
  pending,
}: {
  models: ModelUsage[]
  error: Error | null
  pending: boolean
}) {
  const visible = models.slice(0, 8)
  const maxTotal = Math.max(1, ...visible.map((model) => inputOutputTokens(model.usage)))

  return (
    <div className="mt-5">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-[12px] font-medium text-ink">Last 30 days by model</p>
          <p className="mt-0.5 truncate text-[11px] text-ink-3">ACP agent usage, ranked by input + output tokens.</p>
        </div>
      </div>

      {pending ? (
        <div className="mt-2 space-y-2">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-11" />
          ))}
        </div>
      ) : error ? (
        <p className="mt-2 rounded-control bg-danger/5 px-3 py-2 text-[12px] text-danger">
          Couldn't load model usage: {error.message}
        </p>
      ) : visible.length === 0 ? (
        <p className="mt-2 rounded-control bg-bg/45 px-3 py-2 text-[12px] text-ink-3">
          No ACP model usage recorded yet.
        </p>
      ) : (
        <div className="mt-2 divide-y divide-border/60">
          {visible.map((model) => (
            <ModelUsageRow
              key={`${model.agent ?? ''}:${model.model_provider ?? ''}:${model.model ?? ''}`}
              model={model}
              maxTotal={maxTotal}
            />
          ))}
          {models.length > visible.length ? (
            <div className="pt-2 text-[11px] text-ink-3">
              {models.length - visible.length} more model{models.length - visible.length === 1 ? '' : 's'} with usage.
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}

function ModelUsageRow({ model, maxTotal }: { model: ModelUsage; maxTotal: number }) {
  const total = inputOutputTokens(model.usage)
  const cacheRead = model.usage.cached_input_tokens ?? 0
  const cacheWrite = model.usage.cached_write_tokens ?? 0
  const share = total / maxTotal
  const width = total > 0 ? `${Math.max(3, share * 100)}%` : '0%'
  const sharePercent = share * 100
  const shareLabel = total === 0
    ? '0% of top model'
    : `${sharePercent < 1 ? '<1' : Math.round(sharePercent)}% of top model`

  return (
    <div className="grid gap-3 py-2.5 md:grid-cols-[minmax(0,1fr)_auto]">
      <div className="min-w-0">
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="truncate text-[13px] font-medium text-ink">{modelName(model)}</span>
          <span className="shrink-0 text-[11px] text-ink-3">{formatModelMeta(model)}</span>
        </div>
        <div
          className="mt-1.5 flex items-center gap-2"
          title={`${formatTokens(total)} input + output tokens, ${shareLabel}`}
        >
          <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-bg ring-1 ring-border/60">
            <div className="h-full rounded-full bg-primary" style={{ width }} />
          </div>
          <span className="w-[92px] text-right font-mono text-[10px] text-ink-3 tabular-nums">
            {shareLabel}
          </span>
        </div>
        {cacheRead > 0 || cacheWrite > 0 ? (
          <div className="mt-1 text-[10px] text-ink-3">
            {cacheRead > 0 ? `Cache read ${formatTokens(cacheRead)}` : null}
            {cacheRead > 0 && cacheWrite > 0 ? ' · ' : null}
            {cacheWrite > 0 ? `Cache write ${formatTokens(cacheWrite)}` : null}
          </div>
        ) : null}
      </div>
      <div className="grid grid-cols-3 gap-3 text-right">
        <ModelMetric label="Input" value={model.usage.input_tokens ?? 0} />
        <ModelMetric label="Output" value={model.usage.output_tokens ?? 0} />
        <ModelMetric label="Input + output" value={total} />
      </div>
    </div>
  )
}

function ModelMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="min-w-[72px]">
      <div className="font-mono text-[12px] leading-none text-ink tabular-nums">{formatTokens(value)}</div>
      <div className="mt-1 text-[10px] leading-tight text-ink-3">{label}</div>
    </div>
  )
}

function modelName(model: ModelUsage): string {
  return model.model?.trim() || 'Unknown model'
}

function formatModelMeta(model: ModelUsage): string {
  const parts = [model.agent, model.model_provider].map((part) => part?.trim()).filter(Boolean)
  return parts.length > 0 ? parts.join(' / ') : 'ACP'
}

function UsageSquare({
  cell,
  maxTotal,
  onHover,
}: {
  cell: UsageCell
  maxTotal: number
  onHover: (state: TooltipState | null) => void
}) {
  const day = cell.day
  const dayTotal = day ? inputOutputTokens(day.usage) : 0
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
      aria-label={usageTooltipText(activeDay).replace(/\n/g, ', ')}
      onMouseEnter={show}
      onMouseMove={show}
      onMouseLeave={() => onHover(null)}
      className="cursor-default rounded-[3px] ring-1 ring-border/60 transition-[scale,box-shadow] duration-150 hover:scale-125 hover:ring-ink/25"
      style={{ width: usageSquarePx, height: usageSquarePx, background: levelColor(level) }}
    />
  )
}

function UsageTooltip({ state }: { state: TooltipState }) {
  const left = Math.max(8, Math.min(window.innerWidth - 238, state.x + 14))
  const top = Math.max(8, state.y - 96)
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
        <span className="text-right font-mono tabular-nums">{formatTokens(state.day.usage.input_tokens ?? 0)}</span>
        <span className="text-ink-3">Output</span>
        <span className="text-right font-mono tabular-nums">{formatTokens(state.day.usage.output_tokens ?? 0)}</span>
        <span className="text-ink-3">Input + output</span>
        <span className="text-right font-mono tabular-nums">{formatTokens(inputOutputTokens(state.day.usage))}</span>
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

function usageTooltipText(day: DailyUsage): string {
  return [
    formatUsageDate(day.date),
    `Input ${formatTokens(day.usage.input_tokens ?? 0)}`,
    `Output ${formatTokens(day.usage.output_tokens ?? 0)}`,
    `Input + output ${formatTokens(inputOutputTokens(day.usage))}`,
    `Cache read ${formatTokens(day.usage.cached_input_tokens ?? 0)}`,
    `Cache write ${formatTokens(day.usage.cached_write_tokens ?? 0)}`,
  ].join('\n')
}
