import type { ReactNode } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'

export function AdminSection({
  title,
  action,
  children,
}: {
  title: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="space-y-2.5">
      <div className="flex h-7 items-center justify-between gap-2">
        <h2 className="text-[14px] font-semibold text-ink">{title}</h2>
        {action ? <div className="flex shrink-0 items-center gap-1">{action}</div> : null}
      </div>
      {children}
    </div>
  )
}

// A card of rows for loaded items; until then it shows the load error or progress.
export function AdminList<T>({
  items,
  error,
  empty,
  className = '',
  children,
}: {
  items: T[] | undefined
  error: Error | null
  empty?: string
  className?: string
  children: (item: T) => ReactNode
}) {
  const message = items ? (items.length === 0 ? empty : undefined) : (error?.message ?? 'Loading…')
  return (
    <SettingsCard className={`divide-y divide-border ${className}`}>
      {message ? <p className="px-3 py-6 text-center text-[13px] text-ink-3">{message}</p> : items?.map(children)}
    </SettingsCard>
  )
}
