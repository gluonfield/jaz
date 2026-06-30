import { ChevronDown } from 'lucide-react'
import { type ReactNode, useMemo, useState } from 'react'
import { AnimatedList, AnimatedListItem } from '@/components/ui/AnimatedList'
import { formatTokens } from '@/lib/format/tokens'
import { USAGE_SHARE_OTHER_COLOR, USAGE_SHARE_PALETTE } from '@/lib/usageColors'
import type { UsageTotals } from '@/lib/api/types'
import { type CostSummary, formatUsd, type PricedModel } from '@/lib/usageCost'
import { addUsageTotals, inputTokens, totalUsageTokens, type UsageModelTotals } from '@/lib/usageDaily'

const MODEL_TABLE_COLUMNS = 'minmax(0,1fr) repeat(6, 60px)'

type AgentUsageRow = {
  agent: string
  usage: UsageModelTotals['usage']
  cost: number
  modelCount: number
  pricedCount: number
  unpricedCount: number
}

export function UsageShareCharts({ rows }: { rows: PricedModel[] }) {
  const [modelsExpanded, setModelsExpanded] = useState(false)
  const agents = useMemo(() => buildAgentRows(rows), [rows])
  const slices = useMemo(() => buildAgentPieSlices(agents), [agents])
  const modelEntries = useMemo(() => buildModelPieEntries(rows), [rows])
  const hiddenModelCount = Math.max(0, modelEntries.length - PIE_MAX_SLICES)
  const modelSlices = useMemo(() => buildModelPieSlices(modelEntries, modelsExpanded), [modelEntries, modelsExpanded])
  const agentGrand = slices.reduce((sum, slice) => sum + slice.total, 0)
  const modelGrand = modelSlices.reduce((sum, slice) => sum + slice.total, 0)
  const hasShare = agentGrand > 0 || modelGrand > 0

  return (
    <div className="mt-5">
      <div className="min-w-0">
        <p className="text-[12px] font-medium text-ink">Last 30 days token share</p>
        <p className="mt-0.5 truncate text-[11px] text-ink-3">Hover chart segments or legend items for details.</p>
      </div>

      {!hasShare ? (
        <p className="mt-2 rounded-control bg-bg/45 px-3 py-2 text-[12px] text-ink-3">
          No ACP token usage recorded yet.
        </p>
      ) : (
        <div className="mt-2 grid gap-3 md:grid-cols-2">
          {agentGrand > 0 ? (
            <ShareChartPanel
              title="By coding agent"
              chart={<PieDonut slices={slices} grand={agentGrand} ariaLabelPrefix="Token share by coding agent" />}
              legend={
                <ul className="space-y-1.5">
                  {slices.map((slice) => (
                    <PieLegendRow
                      key={slice.id}
                      slice={slice}
                      grand={agentGrand}
                    />
                  ))}
                </ul>
              }
            />
          ) : null}
          {modelGrand > 0 ? (
            <ShareChartPanel
              title="By model"
              chart={<PieDonut slices={modelSlices} grand={modelGrand} ariaLabelPrefix="Token share by model" />}
              legend={
                <ul className="space-y-1.5">
                  {modelSlices.map((slice) => (
                    <PieLegendRow
                      key={slice.id}
                      slice={slice}
                      grand={modelGrand}
                      onClick={slice.kind === 'other' ? () => setModelsExpanded(true) : undefined}
                      actionLabel={slice.kind === 'other' ? 'Expand grouped models' : undefined}
                    />
                  ))}
                  {modelsExpanded && hiddenModelCount > 0 ? (
                    <LegendCollapseRow onClick={() => setModelsExpanded(false)} />
                  ) : null}
                </ul>
              }
            />
          ) : null}
        </div>
      )}
    </div>
  )
}

export function ModelBreakdown({
  rows,
  cost,
}: {
  rows: PricedModel[]
  cost: CostSummary
}) {
  const visible = rows.slice(0, 8)
  const totalUsage = useMemo(
    () => rows.reduce<UsageTotals>((acc, { model }) => addUsageTotals(acc, model.usage), {}),
    [rows],
  )

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
              <div className="border-b border-border">
                <ModelTableRow
                  label="All models"
                  meta={`${rows.length} total`}
                  usage={totalUsage}
                  cost={cost.priced > 0 ? formatUsd(cost.total) : null}
                />
              </div>
              <div className="divide-y divide-border/60">
                <AnimatedList>
                  {visible.map(({ model, cost: modelCost }) => (
                    <AnimatedListItem
                      key={`${model.agent ?? ''}:${model.model_provider ?? ''}:${model.model ?? ''}`}
                    >
                      <ModelTableRow
                        label={modelName(model)}
                        meta={formatModelMeta(model)}
                        usage={model.usage}
                        cost={modelCost == null ? null : formatUsd(modelCost)}
                      />
                    </AnimatedListItem>
                  ))}
                </AnimatedList>
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
            {cost.unpriced > 0
              ? ` ${cost.unpriced} model${cost.unpriced === 1 ? '' : 's'} without list pricing show no cost.`
              : ''}
          </p>
        </>
      )}
    </div>
  )
}

function ShareChartPanel({
  title,
  chart,
  legend,
}: {
  title: string
  chart: ReactNode
  legend: ReactNode
}) {
  return (
    <div className="rounded-control bg-bg/45 px-3 py-3">
      <p className="text-[12px] font-medium text-ink">{title}</p>
      <div className="mt-3 grid grid-cols-[132px_minmax(0,1fr)] items-start gap-4">
        <div className="justify-self-start">{chart}</div>
        <div className="min-w-0">{legend}</div>
      </div>
    </div>
  )
}

function PieLegendRow({
  slice,
  grand,
  onClick,
  actionLabel,
}: {
  slice: PieSlice
  grand: number
  onClick?: () => void
  actionLabel?: string
}) {
  const rowClassName = [
    'flex min-h-6 w-full items-center gap-2 rounded-[5px] px-1 py-0.5 text-left text-[12px]',
    'outline-none transition-[background-color,color] duration-150 hover:bg-bg/70',
    onClick ? 'cursor-pointer focus-visible:bg-bg/70 focus-visible:ring-1 focus-visible:ring-primary/40' : '',
  ].join(' ')
  const content = (
    <>
      <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: slice.color }} />
      <span className="min-w-0 flex-1 truncate text-ink">{slice.label}</span>
      {onClick ? (
        <ChevronDown size={13} className="-mr-0.5 shrink-0 -rotate-90 text-ink-3" aria-hidden />
      ) : null}
    </>
  )

  return (
    <li className="group relative">
      {onClick ? (
        <button
          type="button"
          className={rowClassName}
          onClick={onClick}
          aria-label={`${actionLabel ?? 'Show details'}: ${sliceDetailLabel(slice, grand)}`}
        >
          {content}
        </button>
      ) : (
        <div className={rowClassName}>
          {content}
        </div>
      )}
      <div className="pointer-events-none absolute left-0 top-full z-tooltip mt-1 hidden w-[260px] group-hover:block group-focus-within:block">
        <PieTooltipCard slice={slice} grand={grand} />
      </div>
    </li>
  )
}

function LegendCollapseRow({ onClick }: { onClick: () => void }) {
  return (
    <li>
      <button
        type="button"
        className="flex min-h-6 w-full items-center gap-2 rounded-[5px] px-1 py-0.5 text-left text-[12px] text-ink-3 outline-none transition-[background-color,color] duration-150 hover:bg-bg/70 hover:text-ink focus-visible:bg-bg/70 focus-visible:text-ink focus-visible:ring-1 focus-visible:ring-primary/40"
        onClick={onClick}
      >
        <span className="size-2.5 shrink-0" />
        <span className="min-w-0 flex-1 truncate">Show fewer</span>
        <ChevronDown size={13} className="-mr-0.5 shrink-0 text-ink-3" aria-hidden />
      </button>
    </li>
  )
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <>
      <span className="text-ink-3">{label}</span>
      <span className="min-w-0 truncate text-right font-mono tabular-nums">{value}</span>
    </>
  )
}

function ModelTableRow({
  label,
  meta,
  usage,
  cost,
}: {
  label: string
  meta: string
  usage: UsageTotals
  cost: string | null
}) {
  return (
    <div
      className="grid items-center gap-x-3 px-1 py-2"
      style={{ gridTemplateColumns: MODEL_TABLE_COLUMNS }}
    >
      <div className="flex min-w-0 items-baseline gap-2">
        <span className="truncate text-[13px] font-medium text-ink">{label}</span>
        <span className="shrink-0 text-[11px] text-ink-3">{meta}</span>
      </div>
      <Cell text={tokenText(inputTokens(usage))} />
      <Cell text={tokenText(usage.cached_input_tokens ?? 0)} tone="muted" />
      <Cell text={tokenText(usage.cached_write_tokens ?? 0)} tone="muted" />
      <Cell text={tokenText(usage.output_tokens ?? 0)} />
      <Cell text={tokenText(totalUsageTokens(usage))} tone="strong" />
      <Cell text={cost} tone="strong" />
    </div>
  )
}

function buildAgentRows(rows: PricedModel[]): AgentUsageRow[] {
  const groups = new Map<string, AgentUsageRow>()
  for (const { model, cost } of rows) {
    if (totalUsageTokens(model.usage) <= 0) continue
    const agent = model.agent?.trim() || 'unknown'
    let group = groups.get(agent)
    if (!group) {
      group = {
        agent,
        usage: {},
        cost: 0,
        modelCount: 0,
        pricedCount: 0,
        unpricedCount: 0,
      }
      groups.set(agent, group)
    }
    group.modelCount += 1
    addUsageTotals(group.usage, model.usage)
    if (cost == null) {
      group.unpricedCount += 1
    } else {
      group.cost += cost
      group.pricedCount += 1
    }
  }
  return [...groups.values()].sort((left, right) => {
    const byTokens = totalUsageTokens(right.usage) - totalUsageTokens(left.usage)
    if (byTokens !== 0) return byTokens
    return left.agent.localeCompare(right.agent)
  })
}

function buildAgentPieSlices(agents: AgentUsageRow[]): PieSlice[] {
  return agents.map((agent, index) => ({
    id: `agent:${agent.agent}`,
    kind: 'agent',
    label: agentName(agent.agent),
    meta: agentMeta(agent),
    total: totalUsageTokens(agent.usage),
    color: AGENT_COLORS[index % AGENT_COLORS.length],
    details: agentDetails(agent),
  }))
}

function Cell({ text, tone = 'default' }: { text: string | null; tone?: 'default' | 'muted' | 'strong' }) {
  const color = tone === 'strong' ? 'text-ink' : tone === 'muted' ? 'text-ink-3' : 'text-ink-2'
  return (
    <span className={`text-right font-mono text-[12px] tabular-nums ${color}`}>
      {text ?? <span className="text-ink-3/55">—</span>}
    </span>
  )
}

function agentName(agent: string): string {
  switch (agent) {
    case 'codex':
      return 'Codex'
    case 'claude':
      return 'Claude'
    case 'grok':
      return 'Grok'
    case 'opencode':
      return 'OpenCode'
    case 'jaz':
      return 'Jaz'
    default:
      return agent === 'unknown' ? 'Unknown agent' : agent
  }
}

function agentMeta(agent: AgentUsageRow): string {
  const priced = agent.pricedCount
  const modelText = `${agent.modelCount} model${agent.modelCount === 1 ? '' : 's'}`
  return agent.unpricedCount > 0
    ? `${modelText} · ${priced} priced`
    : modelText
}

function agentCost(agent: AgentUsageRow): string {
  return agent.pricedCount > 0 ? formatUsd(agent.cost) : '—'
}

function agentDetails(agent: AgentUsageRow): PieSliceDetail[] {
  const reasoning = agent.usage.reasoning_output_tokens ?? 0
  return [
    { label: 'Cost', value: agentCost(agent) },
    { label: 'Input', value: formatTokens(inputTokens(agent.usage)) },
    { label: 'Cache read', value: formatTokens(agent.usage.cached_input_tokens ?? 0) },
    { label: 'Cache write', value: formatTokens(agent.usage.cached_write_tokens ?? 0) },
    { label: 'Output', value: formatTokens(agent.usage.output_tokens ?? 0) },
    ...(reasoning > 0 ? [{ label: 'Reasoning', value: formatTokens(reasoning) }] : []),
    { label: 'Models', value: agentMeta(agent) },
  ]
}

function pctText(pct: number): string {
  return `${pct < 1 ? '<1' : Math.round(pct)}%`
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

type PieSliceDetail = { label: string; value: string }
type PieSliceKind = 'agent' | 'model' | 'other'
type PieSlice = {
  id: string
  kind: PieSliceKind
  label: string
  meta: string
  total: number
  color: string
  details?: PieSliceDetail[]
}
type ModelPieEntry = {
  id: string
  label: string
  meta: string
  total: number
  cost: number | null
  details: PieSliceDetail[]
}

const PIE_SLICE_COLORS = USAGE_SHARE_PALETTE
const AGENT_COLORS = USAGE_SHARE_PALETTE
const PIE_OTHER_COLOR = USAGE_SHARE_OTHER_COLOR
const PIE_MAX_SLICES = 6

function buildModelPieEntries(rows: PricedModel[]): ModelPieEntry[] {
  return rows
    .map(({ model, cost }) => ({
      id: `model:${model.agent ?? ''}:${model.model_provider ?? ''}:${model.model ?? ''}`,
      label: modelName(model),
      meta: formatModelMeta(model),
      total: totalUsageTokens(model.usage),
      cost,
      details: modelDetails(model, cost),
    }))
    .filter((entry) => entry.total > 0)
    .sort((a, b) => b.total - a.total)
}

function buildModelPieSlices(entries: ModelPieEntry[], expanded: boolean): PieSlice[] {
  if (entries.length === 0) return []
  const visible = expanded ? entries : entries.slice(0, PIE_MAX_SLICES)
  const head: PieSlice[] = visible.map((entry, index) => ({
    ...entry,
    kind: 'model',
    color: PIE_SLICE_COLORS[index % PIE_SLICE_COLORS.length],
  }))
  const rest = expanded ? [] : entries.slice(PIE_MAX_SLICES)
  if (rest.length > 0) {
    const pricedCost = rest.reduce((sum, entry) => sum + (entry.cost ?? 0), 0)
    const pricedCount = rest.filter((entry) => entry.cost != null).length
    head.push({
      id: 'model:other',
      kind: 'other',
      label: `${rest.length} other model${rest.length === 1 ? '' : 's'}`,
      meta: 'Click to expand',
      total: rest.reduce((sum, entry) => sum + entry.total, 0),
      color: PIE_OTHER_COLOR,
      details: [
        { label: 'Models', value: String(rest.length) },
        { label: 'Largest', value: rest[0]?.label ?? '—' },
        ...(pricedCount > 0 ? [{ label: 'Cost', value: formatUsd(pricedCost) }] : []),
      ],
    })
  }
  return head
}

function modelDetails(model: UsageModelTotals, cost: number | null): PieSliceDetail[] {
  const reasoning = model.usage.reasoning_output_tokens ?? 0
  return [
    { label: 'Group', value: formatModelMeta(model) },
    { label: 'Cost', value: cost == null ? '—' : formatUsd(cost) },
    { label: 'Input', value: formatTokens(inputTokens(model.usage)) },
    { label: 'Cache read', value: formatTokens(model.usage.cached_input_tokens ?? 0) },
    { label: 'Cache write', value: formatTokens(model.usage.cached_write_tokens ?? 0) },
    { label: 'Output', value: formatTokens(model.usage.output_tokens ?? 0) },
    ...(reasoning > 0 ? [{ label: 'Reasoning', value: formatTokens(reasoning) }] : []),
  ]
}

function PieDonut({
  slices,
  grand,
  ariaLabelPrefix,
}: {
  slices: PieSlice[]
  grand: number
  ariaLabelPrefix: string
}) {
  const [activeIndex, setActiveIndex] = useState<number | null>(null)
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
  const ariaLabel = `${ariaLabelPrefix}: ${slices
    .map((slice) => `${slice.label} ${Math.round((slice.total / grand) * 100)}%`)
    .join(', ')}`
  const active = activeIndex == null ? null : segments[activeIndex]

  return (
    <div className="relative shrink-0">
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        role="group"
        aria-label={ariaLabel}
        className="block"
      >
        <g fill="none" strokeWidth={thickness}>
          {segments.map(({ slice, dash, rotation }, index) => {
            const activeSegment = activeIndex === index
            return (
              <circle
                key={slice.id}
                cx={center}
                cy={center}
                r={radius}
                role="img"
                tabIndex={0}
                aria-label={sliceDetailLabel(slice, grand)}
                stroke={slice.color}
                strokeWidth={activeSegment ? thickness + 3 : thickness}
                strokeDasharray={`${dash} ${circumference}`}
                opacity={activeIndex == null || activeSegment ? 1 : 0.38}
                transform={`rotate(${rotation} ${center} ${center})`}
                className="cursor-default outline-none transition-[opacity,stroke-width] duration-150"
                onMouseEnter={() => setActiveIndex(index)}
                onMouseLeave={() => setActiveIndex(null)}
                onFocus={() => setActiveIndex(index)}
                onBlur={() => setActiveIndex(null)}
              />
            )
          })}
        </g>
        <text
          x={center}
          y={center - 3}
          textAnchor="middle"
          className="pointer-events-none font-mono tabular-nums"
          style={{ fill: 'var(--color-ink)', fontSize: 15 }}
        >
          {formatTokens(grand)}
        </text>
        <text
          x={center}
          y={center + 12}
          textAnchor="middle"
          className="pointer-events-none"
          style={{ fill: 'var(--color-ink-3)', fontSize: 9 }}
        >
          total tokens
        </text>
      </svg>
      {active ? <PieTooltip slice={active.slice} grand={grand} /> : null}
    </div>
  )
}

function PieTooltip({ slice, grand }: { slice: PieSlice; grand: number }) {
  return (
    <div className="pointer-events-none absolute left-1/2 top-full z-tooltip mt-2 w-[260px] -translate-x-1/2">
      <PieTooltipCard slice={slice} grand={grand} />
    </div>
  )
}

function PieTooltipCard({ slice, grand }: { slice: PieSlice; grand: number }) {
  const pct = (slice.total / grand) * 100
  return (
    <div className="rounded-control bg-bg px-3 py-2 text-[11px] text-ink shadow-raised ring-1 ring-border/70">
      <div className="flex items-center gap-2">
        <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: slice.color }} />
        <span className="min-w-0 flex-1 truncate font-medium">{slice.label}</span>
        <span className="font-mono tabular-nums text-ink-2">{pctText(pct)}</span>
      </div>
      <div className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
        <DetailRow label="Total" value={formatTokens(slice.total)} />
        {slice.details?.map((detail) => (
          <DetailRow key={detail.label} label={detail.label} value={detail.value} />
        ))}
        {!slice.details?.length && slice.meta ? <DetailRow label="Group" value={slice.meta} /> : null}
      </div>
    </div>
  )
}

function sliceDetailLabel(slice: PieSlice, grand: number): string {
  const pct = (slice.total / grand) * 100
  return [
    slice.label,
    `${pctText(pct)} of tokens`,
    `${formatTokens(slice.total)} total tokens`,
    ...(slice.details ?? []).map((detail) => `${detail.label} ${detail.value}`),
    ...(!slice.details?.length && slice.meta ? [slice.meta] : []),
  ].join(', ')
}
