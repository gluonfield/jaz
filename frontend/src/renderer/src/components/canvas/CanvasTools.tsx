import { useReactFlow } from '@xyflow/react'
import { useState, type PointerEvent } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import { contains, flows, growToFit, innermostSection, newId, newSection, rectOfNode, rounded, type BoardEdge, type BoardNode, type Rect } from './board'
import { useCanvas } from './canvasContext'
import type { Tool } from './CanvasDock'
import { StickerLayer } from './CanvasStickers'
import { EraserLayer, PenLayer } from './CanvasStroke'

type Make = (x: number, y: number) => Partial<CanvasItem> & Rect

// What each tool puts down, around the point clicked. Text and tables grow
// with what is written, so they start without a height.
const NEW_ITEMS: Partial<Record<Tool, Make>> = {
  sticky: (x, y) => ({ type: 'text', text: '', color: 'yellow', x: x - 100, y: y - 100, width: 200, height: 200 }),
  text: (x, y) => ({ type: 'text', text: '', x, y: y - 20, width: 240, height: 48 }),
  shape: (x, y) => ({ type: 'text', text: '', shape: 'rounded', x: x - 100, y: y - 60, width: 200, height: 120 }),
  timer: (x, y) => ({ type: 'timer', seconds: 300, x: x - 110, y: y - 60, width: 220, height: 120 }),
  table: (x, y) => ({ type: 'table', rows: [['', '', ''], ['', '', ''], ['', '', '']], x, y, width: 420, height: 132 }),
}

// The layer the chosen tool works on, over the board: a click puts the tool's
// item down and a drag draws a section, while the pen, eraser and stickers
// bring their own. `onMade` names an item on its way to the board, and
// `onDone` ends a tool that is used once.
export function ToolLayer({ tool, onMade, onDone }: { tool: Tool; onMade: (id: string) => void; onDone: () => void }) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const { commit, setEditing } = useCanvas()
  const [draft, setDraft] = useState<{ left: number; top: number; x0: number; y0: number; x1: number; y1: number } | null>(null)

  // A new item dropped inside a section grows the section around it.
  const sectionFor = (rect: Rect): Partial<CanvasItem>[] => {
    const sections = flow.getNodes().filter((node) => node.type === 'group').map((node) => ({ id: node.id, rect: rectOfNode(node) }))
    const section = innermostSection(sections, rect.x + rect.width / 2, rect.y + rect.height / 2)
    if (!section || contains(section.rect, rect)) return []
    return [{ id: section.id, ...rounded(growToFit(section.rect, [rect])) }]
  }

  // Empty text opens for typing as soon as it lands.
  const create = (kind: string, make: Make, clientX: number, clientY: number) => {
    const at = flow.screenToFlowPosition({ x: clientX, y: clientY })
    const made = make(at.x, at.y)
    const rect = rounded(made)
    const item: CanvasItem = { id: newId(kind), ...made, ...rect }
    onMade(item.id)
    commit({ nodes: [{ ...item, ...(flows(item) ? { height: null } : {}) }, ...sectionFor(rect)] })
    if (item.type === 'text' && !item.text) setEditing(item.id)
    onDone()
  }

  if (tool === 'pen') return <PenLayer onStroke={(item) => commit({ nodes: [{ id: newId('stroke'), ...item }, ...sectionFor(item)] })} />
  if (tool === 'sticker') return <StickerLayer onPlace={(make, clientX, clientY) => create('sticker', make, clientX, clientY)} />
  if (tool === 'eraser') return <EraserLayer onErase={(id) => flow.getNode(id)?.data.item.locked !== true && commit({ remove: [id] })} />
  const make = NEW_ITEMS[tool]
  if (tool !== 'section' && !make) return null

  const down = (event: PointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    if (make) return
    const { left, top } = event.currentTarget.getBoundingClientRect()
    setDraft({ left, top, x0: event.clientX, y0: event.clientY, x1: event.clientX, y1: event.clientY })
  }
  // A drag draws the section; a click puts down one of a standard size.
  const up = (event: PointerEvent<HTMLDivElement>) => {
    if (make) {
      create(tool, make, event.clientX, event.clientY)
      return
    }
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
