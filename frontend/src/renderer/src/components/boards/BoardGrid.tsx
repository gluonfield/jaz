import { Plus } from 'lucide-react'
import {
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useRef,
  useState,
} from 'react'
import type { BoardLayoutEntry } from '@/lib/api/boards'
import type { Board, BoardItem } from '@/lib/api/types'
import { OpenInMainCue } from './OpenInMainCue'
import { WidgetTile } from './WidgetTile'

const GAP = 12
const MAX_ROWS = 64
const OPEN_CUE_MS = 1600

interface DragState {
  widgetId: string
  mode: 'move' | 'resize'
  startX: number
  startY: number
  dx: number
  dy: number
  origin: { x: number; y: number; w: number; h: number }
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

// Coarse tile grid: tiles snap to whole cells, the user owns placement.
// Pointer-based drag/resize keeps the dependency footprint at zero.
export function BoardGrid({
  board,
  items,
  theme,
  scale,
  onLayoutChange,
  onRemove,
  onNewWidget,
}: {
  board: Board
  items: BoardItem[]
  theme: 'light' | 'dark'
  scale: number
  onLayoutChange: (entry: BoardLayoutEntry) => void
  onRemove: (widgetId: string) => void
  onNewWidget?: () => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  const [drag, setDrag] = useState<DragState | null>(null)
  const dragRef = useRef<DragState | null>(null)
  // Free cell under the pointer; hovering it offers "+ Add widget".
  const [hover, setHover] = useState<{ x: number; y: number } | null>(null)
  // Single board-level cue: a tile in a popped-out window opened in the main
  // app, flashed at the click point. One owner here beats per-tile state.
  const [openCue, setOpenCue] = useState<{ x: number; y: number } | null>(null)
  const cueTimer = useRef<number | null>(null)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver(() => setWidth(el.clientWidth))
    observer.observe(el)
    setWidth(el.clientWidth)
    return () => observer.disconnect()
  }, [])

  useEffect(() => () => {
    if (cueTimer.current) window.clearTimeout(cueTimer.current)
  }, [])

  const showOpenCue = (x: number, y: number) => {
    setOpenCue({ x, y })
    if (cueTimer.current) window.clearTimeout(cueTimer.current)
    cueTimer.current = window.setTimeout(() => setOpenCue(null), OPEN_CUE_MS)
  }

  const cols = board.grid_cols > 0 ? board.grid_cols : 6
  const rowH = board.row_height > 0 ? board.row_height : 120
  const cellW = width > 0 ? (width - GAP * (cols - 1)) / cols : 0

  const cellLeft = (x: number) => x * (cellW + GAP)
  const cellTop = (y: number) => y * (rowH + GAP)
  const tileWidth = (w: number) => w * cellW + (w - 1) * GAP
  const tileHeight = (h: number) => h * rowH + (h - 1) * GAP

  const target = (state: DragState): BoardLayoutEntry => {
    const { origin } = state
    if (state.mode === 'move') {
      return {
        widget_id: state.widgetId,
        x: clamp(Math.round((cellLeft(origin.x) + state.dx) / (cellW + GAP)), 0, cols - origin.w),
        y: clamp(Math.round((cellTop(origin.y) + state.dy) / (rowH + GAP)), 0, MAX_ROWS),
        w: origin.w,
        h: origin.h,
      }
    }
    return {
      widget_id: state.widgetId,
      x: origin.x,
      y: origin.y,
      w: clamp(Math.round((tileWidth(origin.w) + state.dx + GAP) / (cellW + GAP)), 1, cols - origin.x),
      h: clamp(Math.round((tileHeight(origin.h) + state.dy + GAP) / (rowH + GAP)), 1, MAX_ROWS),
    }
  }

  const startDrag = (e: ReactPointerEvent, item: BoardItem, mode: DragState['mode']) => {
    // Buttons and links inside the header keep their click behavior.
    if ((e.target as HTMLElement).closest('button, a')) return
    if (cellW <= 0) return
    e.preventDefault()
    const state: DragState = {
      widgetId: item.widget_id,
      mode,
      startX: e.clientX,
      startY: e.clientY,
      dx: 0,
      dy: 0,
      origin: { x: item.x, y: item.y, w: item.w, h: item.h },
    }
    dragRef.current = state
    setDrag(state)

    const onMove = (ev: PointerEvent) => {
      const current = dragRef.current
      if (!current) return
      const next = { ...current, dx: ev.clientX - current.startX, dy: ev.clientY - current.startY }
      dragRef.current = next
      setDrag(next)
    }
    const onUp = () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      const current = dragRef.current
      dragRef.current = null
      setDrag(null)
      if (!current) return
      const entry = target(current)
      const { origin } = current
      if (
        (entry.x !== origin.x ||
          entry.y !== origin.y ||
          entry.w !== origin.w ||
          entry.h !== origin.h) &&
        // Dropping onto another tile would bury one of them — snap back.
        !overlapsOthers(entry)
      ) {
        onLayoutChange(entry)
      }
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  // The single source of truth for "is this rectangle blocked by a tile".
  const overlapsItems = (x: number, y: number, w: number, h: number, excludeId?: string) =>
    items.some(
      (it) =>
        it.widget_id !== excludeId &&
        x < it.x + it.w &&
        it.x < x + w &&
        y < it.y + it.h &&
        it.y < y + h,
    )
  const overlapsOthers = (entry: BoardLayoutEntry) =>
    overlapsItems(entry.x, entry.y, entry.w, entry.h, entry.widget_id)

  const ghost = drag ? target(drag) : null
  const ghostBlocked = ghost ? overlapsOthers(ghost) : false
  // While dragging the grid extends past the content so every legal cell is
  // visible; at rest it keeps one blank row so "+ Add widget" can start a
  // fresh line below the content, not just fill gaps between tiles.
  const contentRows = Math.max(4, ...items.map((item) => item.y + item.h))
  const rows = drag
    ? Math.max(contentRows + 2, (ghost?.y ?? 0) + (ghost?.h ?? 0) + 2, 8)
    : contentRows + (onNewWidget ? 1 : 0)

  // Track the hovered cell only; whether it is free is derived at render
  // time, so the ghost vanishes by itself when a tile appears under it.
  const onHoverMove = (e: ReactPointerEvent) => {
    if (!onNewWidget) return
    if (dragRef.current) {
      setHover(null)
      return
    }
    const el = containerRef.current
    if (!el || cellW <= 0) return
    const rect = el.getBoundingClientRect()
    const gx = Math.floor((e.clientX - rect.left) / (cellW + GAP))
    const gy = Math.floor((e.clientY - rect.top) / (rowH + GAP))
    const inGrid = gx >= 0 && gx < cols && gy >= 0 && gy < rows
    setHover((prev) =>
      inGrid ? (prev && prev.x === gx && prev.y === gy ? prev : { x: gx, y: gy }) : null,
    )
  }

  return (
    <div
      ref={containerRef}
      className="relative w-full"
      style={{ height: rows * (rowH + GAP) - GAP }}
      onPointerMove={onHoverMove}
      onPointerLeave={() => setHover(null)}
    >
      {drag && cellW > 0 ? (
        <div aria-hidden className="pointer-events-none absolute inset-0">
          {Array.from({ length: rows * cols }, (_, index) => {
            const gx = index % cols
            const gy = Math.floor(index / cols)
            return (
              <div
                key={index}
                className="absolute rounded-card border border-dashed border-border/60"
                style={{ left: cellLeft(gx), top: cellTop(gy), width: cellW, height: rowH }}
              />
            )
          })}
        </div>
      ) : null}
      {ghost ? (
        <div
          className={`absolute rounded-card border-2 border-dashed transition-all duration-75 ${
            ghostBlocked
              ? 'border-danger/60 bg-danger-soft/40'
              : 'border-primary/50 bg-primary-soft/40'
          }`}
          style={{
            left: cellLeft(ghost.x),
            top: cellTop(ghost.y),
            width: tileWidth(ghost.w),
            height: tileHeight(ghost.h),
          }}
        />
      ) : null}
      {onNewWidget && hover && !drag && !overlapsItems(hover.x, hover.y, 1, 1) ? (
        <button
          type="button"
          onClick={onNewWidget}
          className="absolute z-10 flex items-center justify-center gap-1.5 rounded-card border border-dashed border-border/70 text-[12px] text-ink-3 transition-colors duration-150 hover:border-primary/60 hover:text-primary"
          style={{
            left: cellLeft(hover.x),
            top: cellTop(hover.y),
            width: cellW,
            height: rowH,
          }}
        >
          <Plus size={14} />
          Add widget
        </button>
      ) : null}
      {items.map((item) => {
        const isDragging = drag?.widgetId === item.widget_id
        const style: React.CSSProperties = {
          left: cellLeft(item.x),
          top: cellTop(item.y),
          width: tileWidth(item.w),
          height: tileHeight(item.h),
        }
        if (isDragging && drag) {
          if (drag.mode === 'move') {
            style.transform = `translate(${drag.dx}px, ${drag.dy}px)`
          } else {
            style.width = Math.max(cellW, tileWidth(item.w) + drag.dx)
            style.height = Math.max(rowH, tileHeight(item.h) + drag.dy)
          }
        }
        return (
          <div
            key={item.widget_id}
            data-widget-id={item.widget_id}
            className={`absolute ${isDragging ? 'z-20' : ''}`}
            style={style}
          >
            <WidgetTile
              item={item}
              theme={theme}
              scale={scale}
              dragging={isDragging}
              onHeaderPointerDown={(e) => startDrag(e, item, 'move')}
              onResizePointerDown={(e) => startDrag(e, item, 'resize')}
              onRemove={() => onRemove(item.widget_id)}
              onOpenedInMain={showOpenCue}
            />
          </div>
        )
      })}
      <OpenInMainCue point={openCue} />
    </div>
  )
}
