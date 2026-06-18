import { useMemo } from 'react'
import { Skeleton } from '@/components/ui/Skeleton'
import type { ModelUsage } from '@/lib/api/types'
import { formatTokens } from '@/lib/format/tokens'
import { formatUsd, type PricedModel } from '@/lib/usageCost'
import { inputOutputTokens } from '@/lib/usageDaily'

const MODEL_TABLE_COLUMNS = 'minmax(0,1fr) repeat(6, 60px)'

export function ModelBreakdown({
  rows,
  error,
  pending,
  unpriced,
}: {
  rows: PricedModel[]
  error: Error | null
  pending: boolean
  unpriced: number
}) {
  const visible = rows.slice(0, 8)
  const pieModels = useMemo(() => rows.map((row) => row.model), [rows])

  return (
    <div className="mt-5">
      <div className="min-w-0">
        <p className="text-[12px] font-medium text-ink">Last 30 days by model</p>
        <p className="mt-0.5 truncate text-[11px] text-ink-3">ACP agent usage, ranked by input + output tokens.</p>
      </div>

      {pending ? (
        <div className="mt-2 space-y-2">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-10" />
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
        <>
          <div className="mt-2 overflow-x-auto">
            <div className="min-w-[520px]">
              <div
                className="grid gap-x-3 px-1 pb-1.5 text-[10px] font-medium text-ink-3"
                style={{ gridTemplateColumns: MODEL_TABLE_COLUMNS }}
              >
                <span>Model</span>
                <span className="text-right">Input</span>
                <span className="text-right">Cache read</span>
                <span className="text-right">Cache write</span>
                <span className="text-right">Output</span>
                <span className="text-right">Total</span>
                <span className="text-right text-ink-2">Cost</span>
              </div>
              <div className="divide-y divide-border/60">
                {visible.map(({ model, cost }) => (
                  <ModelUsageRow
                    key={`${model.agent ?? ''}:${model.model_provider ?? ''}:${model.model ?? ''}`}
                    model={model}
                    cost={cost}
                  />
                ))}
              </div>
            </div>
          </div>

          {rows.length > visible.length ? (
            <p className="mt-2 text-[11px] text-ink-3">
              {rows.length - visible.length} more model{rows.length - visible.length === 1 ? '' : 's'} with usage.
            </p>
          ) : null}

          <p className="mt-2 text-[11px] text-ink-3">
            Cache read and cache write are already part of Input, not added on top. Total = Input + Output.
          </p>
          <p className="mt-1 text-[11px] text-ink-3">
            Cost is estimated at official per-token list prices (OpenRouter). Coding agents on monthly
            subscriptions bill differently — the real cost is usually lower.
            {unpriced > 0
              ? ` ${unpriced} model${unpriced === 1 ? '' : 's'} without list pricing show no cost.`
              : ''}
          </p>

          <ModelUsagePie models={pieModels} />
        </>
      )}
    </div>
  )
}

function ModelUsageRow({ model, cost }: { model: ModelUsage; cost: number | null }) {
  return (
    <div
      className="grid items-center gap-x-3 px-1 py-2"
      style={{ gridTemplateColumns: MODEL_TABLE_COLUMNS }}
    >
      <div className="flex min-w-0 items-baseline gap-2">
        <span className="truncate text-[13px] font-medium text-ink">{modelName(model)}</span>
        <span className="shrink-0 text-[11px] text-ink-3">{formatModelMeta(model)}</span>
      </div>
      <Cell text={tokenText(model.usage.input_tokens ?? 0)} />
      <Cell text={tokenText(model.usage.cached_input_tokens ?? 0)} tone="muted" />
      <Cell text={tokenText(model.usage.cached_write_tokens ?? 0)} tone="muted" />
      <Cell text={tokenText(model.usage.output_tokens ?? 0)} />
      <Cell text={tokenText(inputOutputTokens(model.usage))} tone="strong" />
      <Cell text={cost == null ? null : formatUsd(cost)} tone="strong" />
    </div>
  )
}

function Cell({ text, tone = 'default' }: { text: string | null; tone?: 'default' | 'muted' | 'strong' }) {
  const color = tone === 'strong' ? 'text-ink' : tone === 'muted' ? 'text-ink-3' : 'text-ink-2'
  return (
    <span className={`text-right font-mono text-[12px] tabular-nums ${color}`}>
      {text ?? <span className="text-ink-3/55">—</span>}
    </span>
  )
}

function tokenText(value: number): string | null {
  return value > 0 ? formatTokens(value) : null
}

function modelName(model: ModelUsage): string {
  return model.model?.trim() || 'Unknown model'
}

function formatModelMeta(model: ModelUsage): string {
  const parts = [model.agent, model.model_provider].map((part) => part?.trim()).filter(Boolean)
  return parts.length > 0 ? parts.join(' / ') : 'ACP'
}

type PieSlice = { label: string; meta: string; total: number; color: string }

const PIE_SLICE_COLORS = [
  'var(--color-primary)',
  'var(--color-accent)',
  'oklch(0.62 0.13 150)',
  'oklch(0.6 0.15 305)',
  'oklch(0.66 0.12 200)',
  'oklch(0.62 0.17 350)',
]
const PIE_OTHER_COLOR = 'var(--color-ink-3)'
const PIE_MAX_SLICES = 6

function buildPieSlices(models: ModelUsage[]): PieSlice[] {
  const ranked = models
    .map((model) => ({
      label: modelName(model),
      meta: formatModelMeta(model),
      total: inputOutputTokens(model.usage),
    }))
    .filter((entry) => entry.total > 0)
    .sort((a, b) => b.total - a.total)
  if (ranked.length === 0) return []
  const head: PieSlice[] = ranked.slice(0, PIE_MAX_SLICES).map((entry, index) => ({
    ...entry,
    color: PIE_SLICE_COLORS[index % PIE_SLICE_COLORS.length],
  }))
  const rest = ranked.slice(PIE_MAX_SLICES)
  if (rest.length > 0) {
    head.push({
      label: `${rest.length} other model${rest.length === 1 ? '' : 's'}`,
      meta: '',
      total: rest.reduce((sum, entry) => sum + entry.total, 0),
      color: PIE_OTHER_COLOR,
    })
  }
  return head
}

function ModelUsagePie({ models }: { models: ModelUsage[] }) {
  const slices = useMemo(() => buildPieSlices(models), [models])
  const grand = slices.reduce((sum, slice) => sum + slice.total, 0)
  if (slices.length === 0 || grand === 0) return null

  return (
    <div className="mt-5 rounded-control bg-bg/45 px-3 py-3">
      <p className="text-[12px] font-medium text-ink">Token share by model</p>
      <p className="mt-0.5 text-[11px] text-ink-3">
        Each model's input + output tokens as a share of the last 30 days.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-x-6 gap-y-4">
        <PieDonut slices={slices} grand={grand} />
        <ul className="min-w-[180px] flex-1 space-y-1.5">
          {slices.map((slice) => {
            const pct = (slice.total / grand) * 100
            return (
              <li key={`${slice.label}:${slice.meta}`} className="flex items-center gap-2 text-[12px]">
                <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: slice.color }} />
                <span className="min-w-0 flex-1 truncate text-ink">
                  {slice.label}
                  {slice.meta ? <span className="text-ink-3"> · {slice.meta}</span> : null}
                </span>
                <span className="shrink-0 font-mono text-[11px] text-ink-2 tabular-nums">
                  {formatTokens(slice.total)}
                </span>
                <span className="w-9 shrink-0 text-right font-mono text-[11px] text-ink-3 tabular-nums">
                  {pct < 1 ? '<1' : Math.round(pct)}%
                </span>
              </li>
            )
          })}
        </ul>
      </div>
    </div>
  )
}

function PieDonut({ slices, grand }: { slices: PieSlice[]; grand: number }) {
  const size = 132
  const center = size / 2
  const radius = 49
  const thickness = 22
  const circumference = 2 * Math.PI * radius
  const gap = slices.length > 1 ? 2 : 0

  let cumulative = 0
  const segments = slices.map((slice) => {
    const fraction = slice.total / grand
    const rotation = -90 + cumulative * 360
    cumulative += fraction
    return { slice, dash: Math.max(0, fraction * circumference - gap), rotation }
  })
  const ariaLabel = `Token share by model: ${slices
    .map((slice) => `${slice.label} ${Math.round((slice.total / grand) * 100)}%`)
    .join(', ')}`

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      role="img"
      aria-label={ariaLabel}
      className="shrink-0"
    >
      <g fill="none" strokeWidth={thickness}>
        {segments.map(({ slice, dash, rotation }) => (
          <circle
            key={`${slice.label}:${slice.meta}`}
            cx={center}
            cy={center}
            r={radius}
            stroke={slice.color}
            strokeDasharray={`${dash} ${circumference}`}
            transform={`rotate(${rotation} ${center} ${center})`}
          />
        ))}
      </g>
      <text
        x={center}
        y={center - 3}
        textAnchor="middle"
        className="font-mono tabular-nums"
        style={{ fill: 'var(--color-ink)', fontSize: 15 }}
      >
        {formatTokens(grand)}
      </text>
      <text
        x={center}
        y={center + 12}
        textAnchor="middle"
        style={{ fill: 'var(--color-ink-3)', fontSize: 9 }}
      >
        total tokens
      </text>
    </svg>
  )
}
