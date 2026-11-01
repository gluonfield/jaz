import type { KeyboardEventHandler, ReactNode } from 'react'

export function SidePanelShell({
  scroll = false,
  className = '',
  onKeyDownCapture,
  children,
}: {
  scroll?: boolean
  className?: string
  onKeyDownCapture?: KeyboardEventHandler<HTMLElement>
  children: ReactNode
}) {
  return (
    <aside
      data-thread-find-shortcuts="off"
      onKeyDownCapture={onKeyDownCapture}
      className={`flex size-full flex-col ${scroll ? 'scrollbar-quiet overflow-y-auto' : 'overflow-hidden'} ${className}`}
    >
      {children}
    </aside>
  )
}
