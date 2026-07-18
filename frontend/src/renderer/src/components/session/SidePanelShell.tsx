import type { KeyboardEventHandler, ReactNode } from 'react'

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
  const sizing = variant === 'fill' ? 'min-h-0 flex-1 overflow-hidden' : 'scrollbar-quiet max-h-full overflow-y-auto'
  return (
    // Phone: ignore the fixed per-view width and fill the full-screen overlay
    // (the inline width still drives the docked desktop column).
    <aside
      data-thread-find-shortcuts="off"
      style={{ width: `var(--side-panel-width, ${width}px)` }}
      onKeyDownCapture={onKeyDownCapture}
      className="flex h-full shrink-0 flex-col bg-bg p-2 max-sm:w-full!"
    >
      <div
        className={`flex flex-col rounded-[14px] bg-surface shadow-sm ring-1 ring-border ${sizing} ${className}`}
      >
        {children}
      </div>
    </aside>
  )
}
