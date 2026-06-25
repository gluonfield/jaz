import type { TargetAndTransition } from 'motion/react'
import type { MouseEvent } from 'react'

// Primitives for the full-screen mobile drawer pattern (sidebar, settings nav,
// session side panel). On phones the drawer is a full-width overlay (CSS
// `max-sm:w-full`) that slides in via transform; on desktop the same element
// animates its real column width.

// Slide target for a drawer's motion wrapper: transform on phones (so width
// never needs measuring), animated column width on desktop. The off-screen
// direction follows the side the drawer is docked to.
export function drawerSlide(opts: {
  isMobile: boolean
  open: boolean
  side: 'left' | 'right'
  width: number
}): TargetAndTransition {
  const { isMobile, open, side, width } = opts
  if (!isMobile) return { width: open ? width : 0 }
  return { x: open ? 0 : side === 'left' ? '-100%' : '100%' }
}

// A full-screen drawer has no visible "outside" to tap, so a tap on any
// non-interactive (empty) area inside it dismisses the drawer the way a backdrop
// scrim would for a partial one. Interactive descendants keep their own behavior.
export function dismissOnEmptyTap(onDismiss: () => void) {
  return (event: MouseEvent<HTMLElement>) => {
    if (!(event.target as HTMLElement).closest('a, button, input, textarea')) onDismiss()
  }
}
