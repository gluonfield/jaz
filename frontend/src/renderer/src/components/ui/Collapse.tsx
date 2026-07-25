import { useState, type ReactNode } from 'react'

// Height reveal for disclosure/accordion content. Animates a grid-rows 0fr→1fr
// transition so the browser interpolates the height itself — this avoids the
// measure-then-snap that animating `height: auto` suffers when nested content
// (e.g. a layout-animated child) reflows mid-transition. Clipping ends once
// expanded so nested layout motion remains visible.
//
// Children stay unmounted until the first open. A clipped-but-present subtree
// still costs full style, layout, and effects, which a transcript pays hundreds
// of times over for tool output nobody has asked to see. Once opened it stays
// mounted, so child state survives collapsing again.
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
  // Latched during render, not in an effect: children must appear in the same
  // commit that opens the track, or the first expand animates an empty box and
  // then snaps as the content lands.
  const [everOpened, setEverOpened] = useState(open)
  if (open && !everOpened) setEverOpened(true)
  return (
    <div
      className={`grid transition-[grid-template-rows] duration-200 ease-out motion-reduce:transition-none ${open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'}`}
      onTransitionEnd={(event) => {
        if (event.target === event.currentTarget && event.propertyName === 'grid-template-rows') setSettledOpen(open)
      }}
    >
      <div className={`min-h-0 min-w-0 ${open && settledOpen ? 'overflow-visible' : 'overflow-hidden'} ${className}`} inert={!open}>
        {everOpened ? children : null}
      </div>
    </div>
  )
}
