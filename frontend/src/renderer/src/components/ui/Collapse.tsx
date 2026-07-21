import { useState, type ReactNode } from 'react'

// Height reveal for disclosure/accordion content. Animates a grid-rows 0fr→1fr
// transition so the browser interpolates the height itself — this avoids the
// measure-then-snap that animating `height: auto` suffers when nested content
// (e.g. a layout-animated child) reflows mid-transition. Content stays mounted;
// clipping ends once expanded so nested layout motion remains visible.
export function Collapse({
  open,
  children,
  className = '',
}: {
  open: boolean
  children: ReactNode
  className?: string
}) {
  const [settledOpen, setSettledOpen] = useState(open)
  return (
    <div
      className={`grid transition-[grid-template-rows] duration-200 ease-out motion-reduce:transition-none ${open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'}`}
      onTransitionEnd={(event) => {
        if (event.target === event.currentTarget && event.propertyName === 'grid-template-rows') setSettledOpen(open)
      }}
    >
      <div className={`min-h-0 ${open && settledOpen ? 'overflow-visible' : 'overflow-hidden'} ${className}`} inert={!open}>
        {children}
      </div>
    </div>
  )
}
