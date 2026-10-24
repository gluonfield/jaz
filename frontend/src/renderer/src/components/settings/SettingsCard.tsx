import type { ReactNode } from 'react'

// The borderless settings card. Jaz's design language delineates regions by
// filled surface tone, not outlines — so the card is just `bg-surface` on the
// page `bg`, never a ring or border. Owning that here keeps every settings
// section consistent and makes a resting border impossible to reintroduce.
// Padding, `overflow-hidden`, and layout come from the caller via `className`.
export function SettingsCard({
  className = '',
  children,
}: {
  className?: string
  children: ReactNode
}) {
  return <div className={`rounded-card bg-surface ${className}`}>{children}</div>
}

// One label/control row inside a card. Rows stack with hairline dividers.
export function SettingsRow({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <div className="grid grid-cols-1 gap-2 border-t border-border px-3 py-2.5 first:border-t-0 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-ink">{title}</p>
        {description ? <p className="mt-0.5 text-[12px] text-ink-3">{description}</p> : null}
      </div>
      <div className="md:justify-self-end">{children}</div>
    </div>
  )
}
