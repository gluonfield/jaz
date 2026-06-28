import { formatTokens } from '@/lib/format/tokens'

export type ShareLegendRow = {
  key: string
  color: string
  label: string
  meta?: string
  total: number
}

// UsageShareLegend renders the swatch/label/tokens/percent rows shared by the
// model and category breakdowns. The caller owns the <ul> layout via className.
export function UsageShareLegend({
  rows,
  grand,
  className,
}: {
  rows: ShareLegendRow[]
  grand: number
  className?: string
}) {
  return (
    <ul className={className}>
      {rows.map((row) => {
        const pct = grand > 0 ? (row.total / grand) * 100 : 0
        return (
          <li key={row.key} className="flex items-center gap-2 text-[12px]">
            <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: row.color }} />
            <span className="min-w-0 flex-1 truncate text-ink">
              {row.label}
              {row.meta ? <span className="text-ink-3"> · {row.meta}</span> : null}
            </span>
            <span className="shrink-0 font-mono text-[11px] text-ink-2 tabular-nums">
              {formatTokens(row.total)}
            </span>
            <span className="w-9 shrink-0 text-right font-mono text-[11px] text-ink-3 tabular-nums">
              {pct < 1 ? '<1' : Math.round(pct)}%
            </span>
          </li>
        )
      })}
    </ul>
  )
}
