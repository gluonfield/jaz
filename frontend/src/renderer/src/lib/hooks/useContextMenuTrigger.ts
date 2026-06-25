import { type MouseEvent, type TouchEvent, useCallback, useEffect, useRef } from 'react'

const LONG_PRESS_MS = 500
const MOVE_CANCEL_PX = 10

type Point = { x: number; y: number }

// Opens a contextual menu via right-click or press-and-hold (touch has no
// right-click) and swallows the lift-off tap. Spread onto the target element.
export function useContextMenuTrigger(onOpen: (point: Point) => void) {
  const timer = useRef<number | null>(null)
  const origin = useRef<Point | null>(null)
  const suppressClick = useRef(false)

  const cancel = useCallback(() => {
    if (timer.current === null) return
    window.clearTimeout(timer.current)
    timer.current = null
  }, [])
  useEffect(() => cancel, [cancel])

  return {
    onContextMenu: (e: MouseEvent) => {
      e.preventDefault()
      onOpen({ x: e.clientX, y: e.clientY })
    },
    onTouchStart: (e: TouchEvent) => {
      const t = e.touches[0]
      if (!t) return
      const { clientX: x, clientY: y } = t
      suppressClick.current = false
      origin.current = { x, y }
      cancel()
      timer.current = window.setTimeout(() => {
        timer.current = null
        suppressClick.current = true
        onOpen({ x, y })
      }, LONG_PRESS_MS)
    },
    onTouchMove: (e: TouchEvent) => {
      const t = e.touches[0]
      if (!t || !origin.current) return
      const dx = t.clientX - origin.current.x
      const dy = t.clientY - origin.current.y
      if (dx * dx + dy * dy > MOVE_CANCEL_PX * MOVE_CANCEL_PX) cancel()
    },
    onTouchEnd: (e: TouchEvent) => {
      cancel()
      if (suppressClick.current) e.preventDefault()
    },
    onTouchCancel: cancel,
    onClick: (e: MouseEvent) => {
      if (!suppressClick.current) return
      suppressClick.current = false
      e.preventDefault()
      e.stopPropagation()
    },
  }
}
