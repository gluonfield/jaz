import { useReactFlow, useStore } from '@xyflow/react'
import { ArrowRight } from 'lucide-react'
import { useState } from 'react'
import { styleOf } from './arrange'
import { OPPOSITE, newId, nextTo, rectOfNode, rounded, type BoardEdge, type BoardNode, type Rect, type Side } from './board'
import { useCanvas } from './canvasContext'
import { ShapeOutline } from './CanvasShapes'
import { lookOf } from './format'

// A dot outside one side of the only selected part. Pointing at it outlines
// where a part like this one would go, a click puts one there joined by a
// connector, and a drag draws a connector as from the side itself.
export function NextDot({ id, side }: { id: string; side: Side }) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const { add } = useCanvas()
  const alone = useStore((state) => state.nodes.filter((node) => node.selected).length === 1)
  const [ghost, setGhost] = useState<Rect | null>(null)
  if (!alone) return null
  const node = flow.getNode(id)!
  const from = rectOfNode(node)
  const { shape } = lookOf(node.data.item)
  const place = () => nextTo(from, side, flow.getNodes().filter((other) => other.id !== id && other.type !== 'group' && other.type !== 'stroke').map(rectOfNode))
  const grow = () => {
    const next = newId(shape ? 'shape' : 'sticky')
    add(
      { id: next, type: 'text', text: '', ...styleOf(node.data.item), ...rounded(place()) },
      [{ id: newId('edge'), fromNode: id, toNode: next, fromSide: side, toSide: OPPOSITE[side] }],
    )
  }
  // The outline is placed from the middle of this side, where the dot hangs.
  const middle = { top: [from.width / 2, 0], right: [from.width, from.height / 2], bottom: [from.width / 2, from.height], left: [0, from.height / 2] }[side]
  return (
    <span className={`canvas-next ${side}`}>
      <button
        type="button"
        aria-label={`Add a part on the ${side}`}
        onPointerEnter={() => setGhost(place())}
        onPointerLeave={() => setGhost(null)}
        onClick={(event) => {
          event.stopPropagation()
          grow()
        }}
      >
        <ArrowRight size={7} strokeWidth={3.5} />
      </button>
      {ghost ? (
        <span className="canvas-ghost" style={{ left: ghost.x - from.x - middle[0], top: ghost.y - from.y - middle[1], width: from.width, height: from.height }}>
          <ShapeOutline shape={shape ?? 'rect'} />
        </span>
      ) : null}
    </span>
  )
}
