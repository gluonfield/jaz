import { useEffect, useRef, type KeyboardEvent, type PointerEvent as ReactPointerEvent } from 'react'

const KEYBOARD_STEP = 24

export function HorizontalResizeHandle({
  label,
  side = 'left',
  width,
  minWidth,
  maxWidth,
  disabled,
  onResizeStart,
  onResize,
  onResizeEnd,
}: {
  label: string
  side?: 'left' | 'right'
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
    if (disabled || event.button !== 0) {
      return
    }
    event.preventDefault()
    cleanupRef.current?.()
    const startX = event.clientX
    const startWidth = width
    const handle = event.currentTarget
    handle.focus()
    const pointerId = event.pointerId
    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    const overlay = document.createElement('div')
    overlay.className = 'fixed inset-0 z-modal cursor-col-resize'
    overlay.setAttribute('aria-hidden', 'true')
    document.body.append(overlay)
    let stopped = false

    const move = (moveEvent: PointerEvent) => {
      moveEvent.preventDefault()
      const delta = moveEvent.clientX - startX
      onResize(clampWidth(startWidth + (side === 'right' ? delta : -delta), minWidth, maxWidth))
    }
    const stop = () => {
      if (stopped) {
        return
      }
      stopped = true
      overlay.remove()
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = previousUserSelect
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
      window.removeEventListener('blur', stop)
      handle.removeEventListener('lostpointercapture', stop)
      if (handle.hasPointerCapture(pointerId)) {
        handle.releasePointerCapture(pointerId)
      }
      cleanupRef.current = null
      onResizeEnd()
    }

    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    handle.setPointerCapture(pointerId)
    onResizeStart()
    cleanupRef.current = stop
    handle.addEventListener('lostpointercapture', stop, { once: true })
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', stop, { once: true })
    window.addEventListener('pointercancel', stop, { once: true })
    window.addEventListener('blur', stop, { once: true })
  }

  const resizeByKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (disabled) {
      return
    }
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') {
      return
    }
    event.preventDefault()
    const direction = (event.key === 'ArrowRight' ? 1 : -1) * (side === 'right' ? 1 : -1)
    onResize(clampWidth(width + direction * KEYBOARD_STEP, minWidth, maxWidth))
  }

  return (
    <div
      role="separator"
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemin={minWidth}
      aria-valuemax={maxWidth}
      aria-valuenow={width}
      tabIndex={disabled ? -1 : 0}
      title="Drag left or right to resize · Arrow keys adjust width"
      onPointerDown={startResize}
      onKeyDown={resizeByKeyboard}
      className={`group absolute inset-y-0 ${side === 'right' ? 'right-0' : '-left-2'} z-shell hidden w-2 cursor-col-resize touch-none items-center justify-center outline-none! sm:flex`}
    >
      <span className={`absolute inset-y-0 ${side === 'right' ? 'right-0' : 'left-0'} w-px bg-transparent transition-colors duration-150 group-hover:bg-primary/50 group-focus-visible:bg-primary`} />
    </div>
  )
}

function clampWidth(width: number, minWidth: number, maxWidth: number): number {
  return Math.round(Math.min(Math.max(width, minWidth), maxWidth))
}
