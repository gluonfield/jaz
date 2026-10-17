import { useReactFlow } from '@xyflow/react'
import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import type { CanvasView } from '@/lib/api/canvas'
import { changedNodeIds, rectOfNode, type BoardEdge, type BoardNode } from './board'

// `inset` is the width of the board's left edge that the chat covers.
export const fitPadding = (inset: number) => ({ padding: { top: '56px', bottom: '96px', left: `${56 + inset}px`, right: '56px' } }) as const
// After this long without input the camera may follow the agent's edits.
const IDLE_MS = 3000

// A server board that differs from what this client last showed carries
// someone else's edits: the user's own are already on screen. Those items
// flash; the camera follows them while the user is idle, otherwise a pill
// offers to.
export function useAgentFollow(view: CanvasView, local: CanvasView, container: RefObject<HTMLDivElement | null>, inset: number) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const touched = useRef(0)
  const shown = useRef<{ view: CanvasView; local: CanvasView } | null>(null)
  const [flash, setFlash] = useState<Record<string, number>>({})
  const [news, setNews] = useState<string[]>([])
  const show = useCallback((ids: string[]) => {
    setNews([])
    requestAnimationFrame(() => void flow.fitView({ nodes: ids.map((id) => ({ id })), duration: 600, maxZoom: 1, ...fitPadding(inset) }))
  }, [flow, inset])

  useEffect(() => {
    const before = shown.current
    shown.current = { view, local }
    if (!before) {
      requestAnimationFrame(() => void flow.fitView({ maxZoom: 1, ...fitPadding(inset) }))
      return
    }
    if (view === before.view) return
    const changed = changedNodeIds(before.local, local)
    if (!changed.length) return
    const now = Date.now()
    setFlash((current) => ({ ...current, ...Object.fromEntries(changed.map((id) => [id, now])) }))
    const box = container.current?.getBoundingClientRect()
    const { x, y, zoom } = flow.getViewport()
    const visible = (id: string) => {
      const node = flow.getNode(id)
      if (!node || !box) return false
      const rect = rectOfNode(node)
      return rect.x * zoom + x >= inset && rect.y * zoom + y >= 0 && (rect.x + rect.width) * zoom + x <= box.width && (rect.y + rect.height) * zoom + y <= box.height
    }
    if (changed.every(visible)) return
    if (now - touched.current > IDLE_MS) show(changed)
    else setNews(changed)
  }, [view, local, flow, show, container, inset])

  const touch = useCallback(() => {
    touched.current = Date.now()
  }, [])
  return { flash, news, show, touch }
}
