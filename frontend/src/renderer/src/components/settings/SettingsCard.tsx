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
