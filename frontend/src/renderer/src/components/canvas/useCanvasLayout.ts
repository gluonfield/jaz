import { useEffect, useEffectEvent, useRef, type RefObject } from 'react'
import { putCanvasLayout, type CanvasPatch, type CanvasView } from '@/lib/api/canvas'
import { flows, rectOfItem, type BoardNode } from './board'
import { layoutIssues, resolvePlaces, type Drawn } from './layout'

const QUIET = 400

// Once the page has drawn and gone quiet, items the agent placed beside others
// get their spot, and the agent gets back what was measured: each item's box
// and anything a reader would trip over.
export function useCanvasLayout({ sessionId, page, local, nodes, hold, apply, container }: {
  sessionId: string
  page: string
  local: CanvasView
  nodes: BoardNode[]
  hold: boolean
  apply: (change: CanvasPatch) => void
  container: RefObject<HTMLDivElement | null>
}) {
  const sent = useRef('')
  const measure = useEffectEvent(() => {
    const sizes = new Map(nodes.map((node) => [node.id, node.measured!]))
    const drawn = local.nodes.flatMap<Drawn>((item) => {
      const size = sizes.get(item.id)
      if (!size) return []
      const fitted = flows(item) ? container.current?.querySelector(`.react-flow__node[data-id="${CSS.escape(item.id)}"] .canvas-fit-content`) : null
      return [{
        id: item.id,
        type: item.type,
        place: item.place,
        x: rectOfItem(item).x,
        y: rectOfItem(item).y,
        width: Math.round(size.width ?? 0),
        height: Math.round(size.height ?? 0),
        scale: fitted ? new DOMMatrix(getComputedStyle(fitted).transform).a : undefined,
      }]
    })
    const { moves, stuck } = resolvePlaces(drawn)
    if (moves.length) {
      apply({ nodes: moves })
      return
    }
    const layout = {
      page,
      items: drawn.map(({ id, x, y, width, height }) => ({ id, x, y, width, height })),
      issues: [...stuck, ...layoutIssues(drawn)],
    }
    const body = JSON.stringify(layout)
    if (body === sent.current) return
    sent.current = body
    putCanvasLayout(sessionId, layout).catch(() => {
      sent.current = ''
    })
  })
  useEffect(() => {
    if (hold || nodes.some((node) => !node.measured)) return
    const timer = setTimeout(measure, QUIET)
    return () => clearTimeout(timer)
  }, [local, nodes, hold])
}
