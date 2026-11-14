import { useReactFlow, useViewport } from '@xyflow/react'
import { useState, type PointerEvent } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import { newId, newSection, rounded, sectionGrowth, type BoardEdge, type BoardNode, type Rect } from './board'
import { useCanvas } from './canvasContext'
import type { Tool } from './CanvasDock'
import { Ghost } from './CanvasShapes'
import { StickerLayer } from './CanvasStickers'
import { PenLayer } from './CanvasStroke'
import { lookOf, type Shape } from './format'

type Make = (x: number, y: number, shape: Shape) => Partial<CanvasItem> & Rect
type Put = (x: number, y: number) => ReturnType<Make>

// What each tool puts down, around the point clicked. Text and tables grow
// with what is written, so they start without a height.
const NEW_ITEMS: Partial<Record<Tool, Make>> = {
  sticky: (x, y) => ({ type: 'text', text: '', color: 'yellow', x: x - 100, y: y - 100, width: 200, height: 200 }),
  text: (x, y) => ({ type: 'text', text: '', x, y: y - 20, width: 240, height: 48 }),
  shape: (x, y, shape) => ({ type: 'text', text: '', shape, x: x - 100, y: y - 60, width: 200, height: 120 }),
  table: (x, y) => ({ type: 'table', rows: [['', '', ''], ['', '', ''], ['', '', '']], x, y, width: 420, height: 132 }),
}

// A click puts the tool's item down, outlined under the pointer until then.
// Plain text has no outline.
function PlaceLayer({ make, onPlace }: { make: Put; onPlace: (clientX: number, clientY: number) => void }) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const { zoom } = useViewport()
  const [hover, setHover] = useState<{ left: number; top: number; x: number; y: number } | null>(null)
  const ghost = () => {
    if (!hover) return null
    const at = flow.screenToFlowPosition(hover)
    const made = make(at.x, at.y)
    const look = lookOf(made)
    if (made.type === 'text' && look.kind === 'text') return null
    const corner = flow.flowToScreenPosition(made)
    return <Ghost shape={look.shape} left={corner.x - hover.left} top={corner.y - hover.top} width={made.width * zoom} height={made.height * zoom} />
  }
  return (
    <div
      className="canvas-tool-layer"
      onPointerDown={(event) => {
        event.preventDefault()
        event.currentTarget.setPointerCapture(event.pointerId)
      }}
      onPointerMove={(event) => {
        const { left, top } = event.currentTarget.getBoundingClientRect()
        setHover({ left, top, x: event.clientX, y: event.clientY })
      }}
      onPointerLeave={() => setHover(null)}
      onPointerUp={(event) => onPlace(event.clientX, event.clientY)}
    >
      {ghost()}
    </div>
  )
}

// A drag draws a section; a click puts down one of a standard size.
function SectionLayer({ onDone }: { onDone: () => void }) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const { commit } = useCanvas()
  const [draft, setDraft] = useState<{ left: number; top: number; x0: number; y0: number; x1: number; y1: number } | null>(null)
  const down = (event: PointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    const { left, top } = event.currentTarget.getBoundingClientRect()
    setDraft({ left, top, x0: event.clientX, y0: event.clientY, x1: event.clientX, y1: event.clientY })
  }
  const up = () => {
    if (!draft) return
    setDraft(null)
    const a = flow.screenToFlowPosition({ x: Math.min(draft.x0, draft.x1), y: Math.min(draft.y0, draft.y1) })
    const b = flow.screenToFlowPosition({ x: Math.max(draft.x0, draft.x1), y: Math.max(draft.y0, draft.y1) })
    const small = Math.abs(draft.x1 - draft.x0) < 12 && Math.abs(draft.y1 - draft.y0) < 12
    commit({ nodes: [newSection(small ? { x: a.x - 240, y: a.y - 160, width: 480, height: 320 } : { x: a.x, y: a.y, width: b.x - a.x, height: b.y - a.y })] })
    onDone()
  }
  return (
    <div
      className="canvas-tool-layer"
      onPointerDown={down}
      onPointerMove={(event) => draft && setDraft({ ...draft, x1: event.clientX, y1: event.clientY })}
      onPointerUp={up}
    >
      {draft ? (
        <div
          className="canvas-section-draft"
          style={{ left: Math.min(draft.x0, draft.x1) - draft.left, top: Math.min(draft.y0, draft.y1) - draft.top, width: Math.abs(draft.x1 - draft.x0), height: Math.abs(draft.y1 - draft.y0) }}
        />
      ) : null}
    </div>
  )
}

// The layer the chosen tool works on, over the board. `shape` is the one the
// shape tool puts down, and `onDone` ends a tool that is used once.
export function ToolLayer({ tool, shape, onDone }: { tool: Tool; shape: Shape; onDone: () => void }) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const { commit, add } = useCanvas()
  const create = (kind: string, make: Put, clientX: number, clientY: number) => {
    const at = flow.screenToFlowPosition({ x: clientX, y: clientY })
    const made = make(at.x, at.y)
    add({ id: newId(kind), ...made, ...rounded(made) })
    onDone()
  }

  if (tool === 'pen') {
    return (
      <PenLayer
        onStroke={(item) => commit({ nodes: [{ id: newId('stroke'), ...item }, ...sectionGrowth(flow.getNodes(), item)] })}
        onErase={(id) => flow.getNode(id)?.data.item.locked !== true && commit({ remove: [id] })}
      />
    )
  }
  if (tool === 'sticker') return <StickerLayer onPlace={(make, clientX, clientY) => create('sticker', make, clientX, clientY)} />
  if (tool === 'section') return <SectionLayer onDone={onDone} />
  const item = NEW_ITEMS[tool]
  if (!item) return null
  const make: Put = (x, y) => item(x, y, shape)
  return <PlaceLayer make={make} onPlace={(clientX, clientY) => create(tool, make, clientX, clientY)} />
}
