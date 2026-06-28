import { useMemo } from 'react'
import { UsageShareLegend } from '@/components/settings/UsageShareLegend'
import { formatTokens } from '@/lib/format/tokens'
import { totalUsageTokens, type UsageCategoryTotals } from '@/lib/usageDaily'

const CATEGORY_META: Record<string, { label: string; color: string }> = {
  chat: { label: 'Chat', color: 'var(--color-primary)' },
  loop_run: { label: 'Loops', color: 'var(--color-accent)' },
  memory_dream: { label: 'Memory Dream', color: 'oklch(0.62 0.13 150)' },
  memory_search: { label: 'Memory Search', color: 'oklch(0.66 0.12 200)' },
  browser_task: { label: 'Browser Agent', color: 'oklch(0.6 0.15 305)' },
}
const CATEGORY_FALLBACK_COLOR = 'var(--color-ink-3)'

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

      <UsageShareLegend
        className="mt-3 grid gap-x-6 gap-y-1.5 sm:grid-cols-2"
        grand={grand}
        rows={segments.map((segment) => ({
          key: segment.category,
          color: segment.color,
          label: segment.label,
          total: segment.total,
        }))}
      />
    </div>
  )
}
