import { useQuery } from '@tanstack/react-query'
import { ChartNoAxesColumn } from 'lucide-react'
import { type MouseEvent, useMemo, useState } from 'react'
import { Skeleton } from '@/components/ui/Skeleton'
import { dailyUsageQuery } from '@/lib/api/sessions'
import type { DailyUsage } from '@/lib/api/types'
import { formatTokens } from '@/lib/format/tokens'
import {
  formatUsageDate,
  peakDay,
  sumUsage,
  totalTokens,
  type UsageCell,
  usageCells,
  usageLevel,
  usageMonthLabels,
} from '@/lib/usageDaily'

type TooltipState = {
  day: DailyUsage
  x: number
  y: number
}

export function UsageSettings() {
  const usage = useQuery(dailyUsageQuery(365))

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
        <UsagePanel days={usage.data} />
      )}
    </section>
  )
}

function UsageSkeleton() {
  return (
    <div className="mt-4 rounded-card bg-surface p-4">
      <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className="h-12" />
        ))}
      </div>
      <Skeleton className="mt-4 h-[130px]" />
    </div>
  )
}

function UsagePanel({ days }: { days: DailyUsage[] }) {
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const cells = useMemo(() => usageCells(days), [days])
  const monthLabels = useMemo(() => usageMonthLabels(cells), [cells])
  const last7 = sumUsage(days.slice(-7))
  const last30 = sumUsage(days.slice(-30))
  const total30 = totalTokens(last30)
  const peak = peakDay(days)
  const activeDays = days.filter((day) => totalTokens(day.usage) > 0).length
  const maxTotal = Math.max(1, ...days.map((day) => totalTokens(day.usage)))
  const hasUsage = days.some((day) => totalTokens(day.usage) > 0)

  return (
    <div className="mt-4 rounded-card bg-surface p-4">
      <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
        <UsageStat label="Last 7 days" value={formatTokens(totalTokens(last7))} />
        <UsageStat label="Last 30 days" value={formatTokens(total30)} />
        <UsageStat label="Input" value={formatTokens(last30.input_tokens ?? 0)} />
        <UsageStat label="Output" value={formatTokens(last30.output_tokens ?? 0)} />
      </div>

      <div className="mt-5 min-w-0">
        <div className="mb-2 flex items-center justify-between gap-4">
          <div className="flex items-center gap-2 text-[12px] font-medium text-ink">
            <ChartNoAxesColumn size={14} className="text-ink-3" />
            <span>Past year</span>
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
            className="grid w-max grid-cols-[24px_auto] gap-x-2 gap-y-1"
            style={{ gridTemplateRows: '16px repeat(7, 10px)' }}
          >
            <div />
            <div
              className="grid h-4 gap-[3px]"
              style={{ gridTemplateColumns: `repeat(${monthLabels.length}, 10px)` }}
            >
              {monthLabels.map((label, index) => (
                <span key={`${label}-${index}`} className="text-[10px] leading-none text-ink-3">
                  {label}
                </span>
              ))}
            </div>

            <div
              className="grid gap-[3px]"
              style={{ gridColumn: 1, gridRow: '2 / span 7', gridTemplateRows: 'repeat(7, 10px)' }}
            >
              {['', 'Mon', '', 'Wed', '', 'Fri', ''].map((label, index) => (
                <div
                  key={`${label}-${index}`}
                  className="h-2.5 text-right text-[10px] leading-[10px] text-ink-3"
                >
                  {label}
                </div>
              ))}
            </div>

            <div
              className="grid grid-flow-col grid-rows-7 gap-[3px]"
              style={{
                gridColumn: 2,
                gridRow: '2 / span 7',
                gridTemplateColumns: `repeat(${monthLabels.length}, 10px)`,
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
          <span>{activeDays} active day{activeDays === 1 ? '' : 's'}</span>
          {peak ? <span>Peak {formatUsageDate(peak.date)}: {formatTokens(totalTokens(peak.usage))}</span> : null}
          {(last30.cached_input_tokens ?? 0) + (last30.cached_write_tokens ?? 0) > 0 ? (
            <span>
              Cache {formatTokens((last30.cached_input_tokens ?? 0) + (last30.cached_write_tokens ?? 0))}
            </span>
          ) : null}
          {(last30.reasoning_output_tokens ?? 0) > 0 ? (
            <span>Reasoning {formatTokens(last30.reasoning_output_tokens ?? 0)}</span>
          ) : null}
        </div>
      </div>

      {tooltip ? <UsageTooltip state={tooltip} /> : null}
    </div>
  )
}

function UsageStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-control bg-bg px-3 py-2">
      <div className="font-mono text-[15px] leading-none text-ink">{value}</div>
      <div className="mt-1 text-[11px] text-ink-3">{label}</div>
    </div>
  )
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
  const dayTotal = day ? totalTokens(day.usage) : 0
  const level = cell.inRange ? usageLevel(dayTotal, maxTotal) : 0

  if (!day) {
    return <span className="size-2.5" />
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
      className="size-2.5 cursor-default rounded-[3px] ring-1 ring-border/60 transition-transform duration-150 hover:scale-125 hover:ring-ink/25"
      style={{ background: levelColor(level) }}
    />
  )
}

function UsageTooltip({ state }: { state: TooltipState }) {
  const left = Math.max(8, Math.min(window.innerWidth - 210, state.x + 14))
  const top = Math.max(8, state.y - 96)

  return (
    <div
      className="pointer-events-none fixed z-tooltip w-[190px] rounded-control bg-ink px-3 py-2 text-[11px] text-bg shadow-raised"
      style={{ left, top }}
    >
      <div className="font-medium">{formatUsageDate(state.day.date)}</div>
      <div className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
        <span className="text-bg/70">Input</span>
        <span className="text-right font-mono">{formatTokens(state.day.usage.input_tokens ?? 0)}</span>
        <span className="text-bg/70">Output</span>
        <span className="text-right font-mono">{formatTokens(state.day.usage.output_tokens ?? 0)}</span>
        <span className="text-bg/70">Total</span>
        <span className="text-right font-mono">{formatTokens(totalTokens(state.day.usage))}</span>
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
    `Total ${formatTokens(totalTokens(day.usage))}`,
  ].join('\n')
}
