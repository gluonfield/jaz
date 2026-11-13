import { useReactFlow, useViewport, type NodeProps } from '@xyflow/react'
import { Highlighter, Slash } from 'lucide-react'
import { useRef, useState, type CSSProperties, type PointerEvent } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import type { BoardNode, Rect } from './board'
import { useCanvas } from './canvasContext'
import { Flash, Resizer } from './CanvasNodes'
import { Menu, Swatches, ToolButton, swatch } from './CanvasMenu'
import { ItemToolbar } from './CanvasToolbar'
import { INKS, after, inkColor, tint } from './format'
import { WEIGHTS, penOf, strokeItem, strokePath, strokePoints, type Pen, type Point } from './stroke'

function inkStyle(pen: Pen, zoom = 1): CSSProperties {
  return {
    stroke: inkColor(tint(INKS, pen.color)) ?? 'var(--color-ink)',
    strokeWidth: (pen.highlight ? pen.weight * 4 : pen.weight) * zoom,
    opacity: pen.highlight ? 0.4 : undefined,
  }
}

function Weight({ weight, onWeight }: { weight: number; onWeight: (weight: number) => void }) {
  return (
    <ToolButton label={`Weight: ${weight}`} onClick={() => onWeight(after<number>(WEIGHTS, weight))}>
      <svg width="18" height="16" aria-hidden><path d="M2 8H16" stroke="currentColor" strokeWidth={weight} strokeLinecap="round" /></svg>
    </ToolButton>
  )
}

// A drawn line. Only its ink takes the pointer, so what lies inside its box
// stays reachable.
export function StrokeNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { commit } = useCanvas()
  const points = strokePoints(data.item)
  const pen = penOf(data.item)
  const path = strokePath(points)
  const set = (change: Partial<CanvasItem>) => commit({ nodes: [{ id, ...change }] })
  return (
    <>
      <Resizer item={data.item} selected={selected} minWidth={1} minHeight={1} />
      {selected ? (
        <ItemToolbar label="Drawing" item={data.item}>
          <Menu label="Ink" face={<span {...swatch('ink', tint(INKS, pen.color))} />}>
            <div className="canvas-toolbar-swatches">
              <Swatches kind="ink" names={INKS} value={tint(INKS, pen.color)} onPick={(color) => set({ color })} />
            </div>
          </Menu>
          <Weight weight={pen.weight} onWeight={(weight) => set({ weight })} />
        </ItemToolbar>
      ) : null}
      <svg className="canvas-stroke" viewBox={`0 0 ${Math.max(1, ...points.map((point) => point[0]))} ${Math.max(1, ...points.map((point) => point[1]))}`} preserveAspectRatio="none">
        <path d={path} vectorEffect="non-scaling-stroke" style={inkStyle(pen)} />
        <path className="canvas-stroke-hit" d={path} vectorEffect="non-scaling-stroke" />
      </svg>
      <Flash id={id} />
    </>
  )
}

// The pen: every drag over the board becomes a line, until another tool is
// chosen.
export function PenLayer({ onStroke }: { onStroke: (item: Partial<CanvasItem> & Rect) => void }) {
  const flow = useReactFlow()
  const { zoom } = useViewport()
  const [pen, setPen] = useState<Pen>({ weight: 4, highlight: false })
  const [straight, setStraight] = useState(false)
  const [trail, setTrail] = useState<Point[] | null>(null)
  const drawn = trail && straight ? [trail[0], trail.at(-1)!] : trail
  const at = (event: PointerEvent<HTMLDivElement>): Point => {
    const box = event.currentTarget.getBoundingClientRect()
    return [event.clientX - box.left, event.clientY - box.top]
  }
  const finish = (event: PointerEvent<HTMLDivElement>) => {
    if (!drawn) return
    const box = event.currentTarget.getBoundingClientRect()
    setTrail(null)
    onStroke(strokeItem(drawn.map(([x, y]): Point => {
      const point = flow.screenToFlowPosition({ x: x + box.left, y: y + box.top })
      return [point.x, point.y]
    }), pen))
  }
  const change = (next: Partial<Pen>) => setPen((current) => ({ ...current, ...next }))
  return (
    <>
      <div
        className="canvas-tool-layer canvas-pen-layer"
        onPointerDown={(event) => {
          event.preventDefault()
          event.currentTarget.setPointerCapture(event.pointerId)
          setTrail([at(event)])
        }}
        onPointerMove={(event) => {
          const point = at(event)
          setTrail((current) => current && [...current, point])
        }}
        onPointerUp={finish}
        onPointerCancel={() => setTrail(null)}
      >
        {drawn ? <svg className="canvas-pen-trail"><path d={strokePath(drawn)} style={inkStyle(pen, zoom)} /></svg> : null}
      </div>
      <div className="canvas-tool-options canvas-toolbar canvas-overlay" role="toolbar" aria-label="Pen">
        <Swatches kind="ink" names={INKS} value={tint(INKS, pen.color)} onPick={(color) => change({ color: color ?? undefined })} />
        <span className="canvas-toolbar-rule" />
        <Weight weight={pen.weight} onWeight={(weight) => change({ weight })} />
        <ToolButton label="Highlighter" on={pen.highlight} onClick={() => change({ highlight: !pen.highlight })}><Highlighter size={16} /></ToolButton>
        <ToolButton label="Straight line" on={straight} onClick={() => setStraight(!straight)}><Slash size={16} /></ToolButton>
      </div>
    </>
  )
}

// The eraser removes each line it is dragged across.
export function EraserLayer({ onErase }: { onErase: (id: string) => void }) {
  const gone = useRef(new Set<string>())
  const erase = (event: PointerEvent<HTMLDivElement>) => {
    if (!(event.buttons & 1)) return
    if (event.type === 'pointerdown') gone.current.clear()
    for (const element of document.elementsFromPoint(event.clientX, event.clientY)) {
      const id = element.classList.contains('canvas-stroke-hit') ? element.closest('.react-flow__node')?.getAttribute('data-id') : null
      if (id && !gone.current.has(id)) {
        gone.current.add(id)
        onErase(id)
      }
    }
  }
  return <div className="canvas-tool-layer canvas-eraser-layer" onPointerDown={erase} onPointerMove={erase} />
}
