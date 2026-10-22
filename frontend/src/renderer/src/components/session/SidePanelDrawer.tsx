import { spring } from 'motion'
import { motion, useReducedMotion } from 'motion/react'
import type { CSSProperties, ReactNode } from 'react'
import { HorizontalResizeHandle } from '@/components/ui/HorizontalResizeHandle'
import type { useSidePanelState } from '@/components/session/SidePanelState'

export const panelSpring = { stiffness: 400, damping: 36 }
const widthTransition = `width ${spring({ ...panelSpring, keyframes: [0, 1] })}`

export function SidePanelDrawer({ panel, isMobile, children }: {
  panel: ReturnType<typeof useSidePanelState>
  isMobile: boolean
  children: ReactNode
}) {
  const reducedMotion = useReducedMotion()
  return <motion.div
    style={{
      '--side-panel-width': '100%',
      width: isMobile ? undefined : panel.open ? panel.widthStyle : 0,
      transition: isMobile || panel.resizing || reducedMotion ? 'none' : widthTransition,
    } as CSSProperties}
    className="relative h-full shrink-0 overflow-hidden max-sm:absolute max-sm:inset-y-0 max-sm:right-0 max-sm:z-shell max-sm:w-full!"
    inert={!panel.open}
    aria-hidden={!panel.open}
    initial={false}
    animate={{ x: isMobile && !panel.open ? '100%' : 0 }}
    transition={panel.resizing || reducedMotion ? { duration: 0 } : { type: 'spring', ...panelSpring }}
  >
    {panel.resizable ? <HorizontalResizeHandle
      label="Resize side panel"
      width={panel.width}
      minWidth={panel.minWidth}
      maxWidth={panel.maxWidth}
      disabled={isMobile || !panel.open}
      onResizeStart={() => panel.setResizing(true)}
      onResize={panel.resize}
      onResizeEnd={() => panel.setResizing(false)}
    /> : null}
    {children}
  </motion.div>
}
