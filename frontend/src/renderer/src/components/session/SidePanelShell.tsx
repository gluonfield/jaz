import type { KeyboardEventHandler, ReactNode } from 'react'

const SIDE_PANEL_SURFACE_CLASS =
  'flex h-full shrink-0 flex-col border-l border-border bg-surface shadow-[-12px_0_28px_-28px_rgba(18,20,30,0.36)] max-sm:w-full! max-sm:border-l-0 dark:shadow-none'

export function SidePanelShell({
  width,
  variant = 'fill',
  className = '',
  onKeyDownCapture,
  children,
}: {
  width: number
  variant?: 'fill' | 'hug'
  className?: string
  onKeyDownCapture?: KeyboardEventHandler<HTMLElement>
  children: ReactNode
}) {
  const sizing =
    variant === 'fill' ? 'min-h-0 flex-1 overflow-hidden' : 'scrollbar-quiet max-h-full overflow-y-auto'
  return (
    <aside
      data-thread-find-shortcuts="off"
      style={{ width: `var(--side-panel-width, ${width}px)` }}
      onKeyDownCapture={onKeyDownCapture}
      className={`${SIDE_PANEL_SURFACE_CLASS} ${sizing} ${className}`}
    >
      {children}
    </aside>
  )
}
