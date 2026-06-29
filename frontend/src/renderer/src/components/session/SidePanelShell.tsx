import type { ReactNode } from 'react'

export function SidePanelShell({
  width,
  variant = 'fill',
  className = '',
  children,
}: {
  width: number
  variant?: 'fill' | 'hug'
  className?: string
  children: ReactNode
}) {
  const sizing = variant === 'fill' ? 'min-h-0 flex-1 overflow-hidden' : 'max-h-full overflow-y-auto'
  return (
    // Phone: ignore the fixed per-view width and fill the full-screen overlay
    // (the inline width still drives the docked desktop column).
    <aside
      style={{ width: `var(--side-panel-width, ${width}px)` }}
      className="flex h-full shrink-0 flex-col bg-bg p-2 max-sm:w-full!"
    >
      <div
        className={`flex flex-col rounded-[14px] bg-surface shadow-[var(--shadow-raised)] ring-1 ring-border ${sizing} ${className}`}
      >
        {children}
      </div>
    </aside>
  )
}
