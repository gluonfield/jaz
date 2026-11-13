import { createContext, use, useState, type MouseEvent, type ReactNode } from 'react'

// Pressing a control must not take the focus from the text being edited.
export const hold = (event: MouseEvent) => event.preventDefault()

// The label of the one open menu.
const OpenMenu = createContext<[string | null, (label: string | null) => void]>([null, () => {}])

export function Menus({ children }: { children: ReactNode }) {
  return <OpenMenu value={useState<string | null>(null)}>{children}</OpenMenu>
}

export function ToolButton({ label, on, onClick, children }: { label: string; on?: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" className={on ? 'on' : ''} aria-label={label} aria-pressed={on} title={label} onClick={onClick}>
      {children}
    </button>
  )
}

// A menu closes on a choice, unless it is told to stay for more.
export function Menu({ label, face, stay, children }: { label: string; face: ReactNode; stay?: boolean; children: ReactNode }) {
  const [current, setCurrent] = use(OpenMenu)
  const open = current === label
  return (
    <div className="canvas-toolbar-menu">
      <button type="button" className={open ? 'on' : ''} aria-label={label} aria-expanded={open} title={label} onClick={() => setCurrent(open ? null : label)}>
        {face}
      </button>
      {open ? <div className="canvas-toolbar-panel" role="menu" aria-label={label} onClick={() => stay || setCurrent(null)}>{children}</div> : null}
    </div>
  )
}
