import { useMemo } from 'react'
import { formatTokens } from '@/lib/format/tokens'
import { USAGE_SHARE_OTHER_COLOR, USAGE_SHARE_PALETTE } from '@/lib/usageColors'
import { totalUsageTokens, type UsageCategoryTotals } from '@/lib/usageDaily'

// Each activity gets a fixed hue from the shared palette so its color is stable
// regardless of token rank, and consistent with the agent/model pies.
const CATEGORY_META: Record<string, { label: string; color: string }> = {
  chat: { label: 'Chat', color: USAGE_SHARE_PALETTE[0] },
  loop_run: { label: 'Loops', color: USAGE_SHARE_PALETTE[1] },
  memory_dream: { label: 'Memory Dream', color: USAGE_SHARE_PALETTE[2] },
  memory_search: { label: 'Memory Search', color: USAGE_SHARE_PALETTE[3] },
  browser_task: { label: 'Browser Agent', color: USAGE_SHARE_PALETTE[4] },
  memory_source: { label: 'Memory Capture', color: USAGE_SHARE_PALETTE[5] },
}
const CATEGORY_FALLBACK_COLOR = USAGE_SHARE_OTHER_COLOR

function categoryMeta(category: string): { label: string; color: string } {
  return CATEGORY_META[category] ?? { label: category, color: CATEGORY_FALLBACK_COLOR }
}

export function CategoryBreakdown({ categories }: { categories: UsageCategoryTotals[] }) {
  const segments = useMemo(
    () =>
      categories
        .map((category) => ({
          category: category.category,
          ...categoryMeta(category.category),
          total: totalUsageTokens(category.usage),
        }))
        .filter((segment) => segment.total > 0),
    [categories],
  )
  const grand = segments.reduce((sum, segment) => sum + segment.total, 0)
  if (segments.length === 0 || grand === 0) return null

  return (
    <div className="mt-5 rounded-control bg-bg/45 px-3 py-3">
      <p className="text-[12px] font-medium text-ink">Last 30 days by activity</p>
      <p className="mt-0.5 text-[11px] text-ink-3">
        Where tokens went across chat, loops, memory, and the browser agent.
      </p>

      <div className="mt-3 flex h-2.5 w-full overflow-hidden rounded-full ring-1 ring-border/60">
        {segments.map((segment) => (
          <div
            key={segment.category}
            style={{ width: `${(segment.total / grand) * 100}%`, background: segment.color }}
            title={`${segment.label}: ${formatTokens(segment.total)}`}
          />
        ))}
      </div>

      <ul className="mt-3 grid gap-x-6 gap-y-1.5 sm:grid-cols-2">
        {segments.map((segment) => {
          const pct = (segment.total / grand) * 100
          return (
            <li key={segment.category} className="flex items-center gap-2 text-[12px]">
              <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: segment.color }} />
              <span className="min-w-0 flex-1 truncate text-ink">{segment.label}</span>
              <span className="shrink-0 font-mono text-[11px] text-ink-2 tabular-nums">
                {formatTokens(segment.total)}
              </span>
              <span className="w-9 shrink-0 text-right font-mono text-[11px] text-ink-3 tabular-nums">
                {pct < 1 ? '<1' : Math.round(pct)}%
              </span>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
