import { useMemo } from 'react'
import { UsageShareLegend } from '@/components/settings/UsageShareLegend'
import { formatTokens } from '@/lib/format/tokens'
import { formatUsd, type PricedModel } from '@/lib/usageCost'
import { inputTokens, totalUsageTokens, type UsageModelTotals } from '@/lib/usageDaily'

const MODEL_TABLE_COLUMNS = 'minmax(0,1fr) repeat(6, 60px)'

export function ModelBreakdown({
  rows,
  unpriced,
}: {
  rows: PricedModel[]
  unpriced: number
}) {
  const visible = rows.slice(0, 8)
  const pieModels = useMemo(() => rows.map((row) => row.model), [rows])

  return (
    <div className="mt-5">
      <div className="min-w-0">
        <p className="text-[12px] font-medium text-ink">Last 30 days by model</p>
        <p className="mt-0.5 truncate text-[11px] text-ink-3">ACP agent usage, ranked by total tokens.</p>
      </div>

      {visible.length === 0 ? (
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
            Total = Input + Cache read + Cache write + Output.
          </p>
          <p className="mt-1 text-[11px] text-ink-3">
            Cost is an OpenRouter list-price equivalent for subscription-backed coding-agent usage,
            not the actual subscription bill.
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

function ModelUsageRow({ model, cost }: { model: UsageModelTotals; cost: number | null }) {
  return (
    <div
      className="grid items-center gap-x-3 px-1 py-2"
      style={{ gridTemplateColumns: MODEL_TABLE_COLUMNS }}
    >
      <div className="flex min-w-0 items-baseline gap-2">
        <span className="truncate text-[13px] font-medium text-ink">{modelName(model)}</span>
        <span className="shrink-0 text-[11px] text-ink-3">{formatModelMeta(model)}</span>
      </div>
      <Cell text={tokenText(inputTokens(model.usage))} />
      <Cell text={tokenText(model.usage.cached_input_tokens ?? 0)} tone="muted" />
      <Cell text={tokenText(model.usage.cached_write_tokens ?? 0)} tone="muted" />
      <Cell text={tokenText(model.usage.output_tokens ?? 0)} />
      <Cell text={tokenText(totalUsageTokens(model.usage))} tone="strong" />
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

function modelName(model: UsageModelTotals): string {
  return model.model?.trim() || 'Unknown model'
}

function formatModelMeta(model: UsageModelTotals): string {
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

function buildPieSlices(models: UsageModelTotals[]): PieSlice[] {
  const ranked = models
    .map((model) => ({
      label: modelName(model),
      meta: formatModelMeta(model),
      total: totalUsageTokens(model.usage),
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

function ModelUsagePie({ models }: { models: UsageModelTotals[] }) {
  const slices = useMemo(() => buildPieSlices(models), [models])
  const grand = slices.reduce((sum, slice) => sum + slice.total, 0)
  if (slices.length === 0 || grand === 0) return null

  return (
    <div className="mt-5 rounded-control bg-bg/45 px-3 py-3">
      <p className="text-[12px] font-medium text-ink">Token share by model</p>
      <p className="mt-0.5 text-[11px] text-ink-3">
        Each model's total tokens as a share of the last 30 days.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-x-6 gap-y-4">
        <PieDonut slices={slices} grand={grand} />
        <UsageShareLegend
          className="min-w-[180px] flex-1 space-y-1.5"
          grand={grand}
          rows={slices.map((slice) => ({
            key: `${slice.label}:${slice.meta}`,
            color: slice.color,
            label: slice.label,
            meta: slice.meta,
            total: slice.total,
          }))}
        />
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
