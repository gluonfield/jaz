import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, X } from 'lucide-react'
import { useState } from 'react'
import { evaluateSpendStatus, usageSummaryQuery } from '@/lib/api/cloudUsage'
import { useHasCloud } from '@/lib/cloudSession'

// Subtle spend-cap indicator above the Cloud composer. Stays silent on local
// backends and below the warning threshold.
export function CloudSpendBanner({
  capError,
  onClearError,
}: {
  capError?: string | null
  onClearError?: () => void
}) {
  const hasCloud = useHasCloud()
  const summary = useQuery({ ...usageSummaryQuery, enabled: hasCloud })
  const [dismissedKey, setDismissedKey] = useState('')

  if (!hasCloud) return null

  const status = evaluateSpendStatus(summary.data)
  if (!capError && !status) return null

  const exceeded = Boolean(capError) || status?.level === 'exceeded'
  const message = capError || status?.message || 'Monthly spend cap reached.'
  const dismissKey = `${summary.data?.month_start ?? ''}:${status?.percent ?? 0}`
  if (!exceeded && dismissedKey === dismissKey) return null

  return (
    <div
      role={exceeded ? 'alert' : 'status'}
      className={`mb-2 flex items-center gap-2 rounded-control px-3 py-1.5 text-[12px] tabular-nums ${
        exceeded
          ? 'bg-danger-soft text-danger ring-1 ring-danger/25'
          : 'bg-accent-soft text-accent-strong ring-1 ring-accent/25'
      }`}
    >
      <AlertTriangle size={13} className="shrink-0" />
      <span className="min-w-0 flex-1">{message}</span>
      {!exceeded ? (
        <button
          type="button"
          aria-label="Dismiss spend warning"
          onClick={() => setDismissedKey(dismissKey)}
          className="grid size-5 shrink-0 cursor-pointer place-items-center rounded-full opacity-75 transition-opacity duration-150 hover:opacity-100"
        >
          <X size={12} />
        </button>
      ) : onClearError ? (
        <button
          type="button"
          aria-label="Dismiss notice"
          onClick={onClearError}
          className="grid size-5 shrink-0 cursor-pointer place-items-center rounded-full opacity-75 transition-opacity duration-150 hover:opacity-100"
        >
          <X size={12} />
        </button>
      ) : null}
    </div>
  )
}
