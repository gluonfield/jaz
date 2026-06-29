import { useEffect, useRef, type KeyboardEvent, type PointerEvent as ReactPointerEvent } from 'react'

const KEYBOARD_STEP = 24

export function SidePanelResizeHandle({
  width,
  minWidth,
  maxWidth,
  disabled,
  onResizeStart,
  onResize,
  onResizeEnd,
}: {
  width: number
  minWidth: number
  maxWidth: number
  disabled: boolean
  onResizeStart: () => void
  onResize: (width: number) => void
  onResizeEnd: () => void
}) {
  const cleanupRef = useRef<(() => void) | null>(null)

  useEffect(() => () => cleanupRef.current?.(), [])

  const startResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (disabled || event.button !== 0) return
    event.preventDefault()
    cleanupRef.current?.()
    const startX = event.clientX
    const startWidth = width
    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    let stopped = false

    const move = (moveEvent: PointerEvent) => {
      moveEvent.preventDefault()
      onResize(clampWidth(startWidth + startX - moveEvent.clientX, minWidth, maxWidth))
    }
    const stop = () => {
      if (stopped) return
      stopped = true
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = previousUserSelect
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
      cleanupRef.current = null
      onResizeEnd()
    }

    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    onResizeStart()
    cleanupRef.current = stop
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', stop, { once: true })
    window.addEventListener('pointercancel', stop, { once: true })
  }

  const resizeByKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (disabled) return
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    const direction = event.key === 'ArrowLeft' ? 1 : -1
    onResize(clampWidth(width + direction * KEYBOARD_STEP, minWidth, maxWidth))
  }

  return (
    <div
      role="separator"
      aria-label="Resize side panel"
      aria-orientation="vertical"
      aria-valuemin={minWidth}
      aria-valuemax={maxWidth}
      aria-valuenow={width}
      tabIndex={disabled ? -1 : 0}
      title="Drag to resize side panel"
      onPointerDown={startResize}
      onKeyDown={resizeByKeyboard}
      className="group absolute inset-y-0 left-0 z-shell hidden w-4 cursor-col-resize touch-none items-stretch justify-center outline-none sm:flex"
    >
      <span className="my-3 w-px rounded-full bg-transparent transition-colors duration-150 group-hover:bg-primary/50 group-focus-visible:bg-primary" />
    </div>
  )
}

function clampWidth(width: number, minWidth: number, maxWidth: number): number {
  return Math.round(Math.min(Math.max(width, minWidth), maxWidth))
}
