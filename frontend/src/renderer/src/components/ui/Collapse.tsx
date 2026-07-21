import type { ReactNode } from 'react'

// Height reveal for disclosure/accordion content. Animates a grid-rows 0fr→1fr
// transition so the browser interpolates the height itself — this avoids the
// measure-then-snap that animating `height: auto` suffers when nested content
// (e.g. a layout-animated child) reflows mid-transition. Content stays mounted so
// both directions animate; `inert` drops it from tab/focus order while collapsed.
export function Collapse({
  open,
  children,
  className = '',
}: {
  open: boolean
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={`grid transition-[grid-template-rows] duration-200 ease-out motion-reduce:transition-none ${open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'}`}
    >
      <div className={`min-h-0 overflow-hidden ${className}`} inert={!open}>
        {children}
      </div>
    </div>
  )
}
