import { Check } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type CSSProperties, type ReactNode, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { layoutPoint, layoutRect, layoutViewport } from '@/lib/dom/zoom'

const GAP = 6

// no-drag keeps the panel clickable when it overlaps the titlebar drag region.
const menuPanelClass =
  'min-w-[176px] rounded-[14px] bg-surface p-1.5 shadow-xl ring-1 ring-border [-webkit-app-region:no-drag]'

// A floating menu anchored to its trigger, dismissed on outside-click/Escape.
// The panel is portaled and fixed-positioned to the trigger so it can't be
// clipped by a scroll/overflow ancestor (e.g. a modal body); the anchor carries
// `data-escape-surface` while open so a host modal lets Escape close the menu
// first instead of closing itself.
export function Popover({
  open,
  onClose,
  trigger,
  children,
  placement = 'above',
  align = 'start',
}: {
  open: boolean
  onClose: () => void
  trigger: ReactNode
  children: ReactNode
  placement?: 'above' | 'below'
  // 'end' anchors the panel to the trigger's right edge, for triggers near
  // the window's right side.
  align?: 'start' | 'end'
}) {
  const anchorRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const reducedMotion = useReducedMotion()
  const [rect, setRect] = useState<DOMRect | null>(null)

  useLayoutEffect(() => {
    if (open && anchorRef.current) setRect(layoutRect(anchorRef.current))
  }, [open])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (anchorRef.current?.contains(t) || menuRef.current?.contains(t)) return
      onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.stopPropagation()
      onClose()
    }
    // Fixed to the trigger: close rather than chase it when an outer surface
    // scrolls or the window resizes. Scrolling inside the menu stays open.
    const onScroll = (e: Event) => {
      if (menuRef.current?.contains(e.target as Node)) return
      onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', onClose)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', onClose)
    }
  }, [open, onClose])

  const slide = reducedMotion ? 0 : placement === 'above' ? 6 : -6
  let style: CSSProperties = {}
  if (rect) {
    const vp = layoutViewport()
    style = {
      position: 'fixed',
      zIndex: 'var(--z-modal)',
      ...(placement === 'below' ? { top: rect.bottom + GAP } : { bottom: vp.height - rect.top + GAP }),
      ...(align === 'end' ? { right: vp.width - rect.right } : { left: rect.left }),
    }
  }

  return (
    <div ref={anchorRef} className="relative" data-escape-surface={open ? '' : undefined}>
      {trigger}
      {createPortal(
        <AnimatePresence>
          {open && rect ? (
            <motion.div
              ref={menuRef}
              data-escape-surface=""
              initial={{ opacity: 0, y: slide }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: slide }}
              transition={{ duration: 0.15, ease: 'easeOut' }}
              style={style}
              className={menuPanelClass}
            >
              {children}
            </motion.div>
          ) : null}
        </AnimatePresence>,
        document.body,
      )}
    </div>
  )
}

// Like Popover, but anchored to a cursor point and clamped to the viewport.
export function ContextMenu({
  point,
  onClose,
  children,
}: {
  point: { x: number; y: number }
  onClose: () => void
  children: ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)
  const reducedMotion = useReducedMotion()
  const [pos, setPos] = useState(point)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const { width, height } = layoutRect(el)
    const vp = layoutViewport()
    const p = layoutPoint(point.x, point.y)
    const margin = 8
    setPos({
      x: Math.max(margin, Math.min(p.x, vp.width - width - margin)),
      y: Math.max(margin, Math.min(p.y, vp.height - height - margin)),
    })
  }, [point])

  useEffect(() => {
    const onOutside = (e: Event) => {
      if (ref.current?.contains(e.target as Node)) return
      onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.stopPropagation()
      onClose()
    }
    document.addEventListener('mousedown', onOutside)
    document.addEventListener('touchstart', onOutside)
    document.addEventListener('keydown', onKey)
    window.addEventListener('scroll', onOutside, true)
    window.addEventListener('resize', onClose)
    return () => {
      document.removeEventListener('mousedown', onOutside)
      document.removeEventListener('touchstart', onOutside)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('scroll', onOutside, true)
      window.removeEventListener('resize', onClose)
    }
  }, [onClose])

  return createPortal(
    <motion.div
      ref={ref}
      data-escape-surface=""
      initial={{ opacity: 0, scale: reducedMotion ? 1 : 0.96 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ duration: 0.12, ease: 'easeOut' }}
      style={{ position: 'fixed', top: pos.y, left: pos.x, zIndex: 'var(--z-modal)' }}
      className={menuPanelClass}
    >
      {children}
    </motion.div>,
    document.body,
  )
}

export function MenuRow({
  selected,
  disabled,
  onClick,
  children,
}: {
  selected?: boolean
  disabled?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 enabled:hover:bg-surface-2 disabled:cursor-default disabled:opacity-50 ${
        selected ? 'text-ink' : 'text-ink-2'
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {selected ? <Check size={13} className="shrink-0 text-primary" /> : null}
    </button>
  )
}
