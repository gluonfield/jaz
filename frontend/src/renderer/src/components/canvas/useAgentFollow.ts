import { getViewportForBounds, useReactFlow, useStoreApi } from '@xyflow/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { CanvasView } from '@/lib/api/canvas'
import { changedNodeIds, rectOfNode, type BoardEdge, type BoardNode } from './board'

// `inset` is the width of the board's left edge that the chat covers.
export const fitPadding = (inset: number) => ({ padding: { top: '56px', bottom: '96px', left: `${56 + inset}px`, right: '56px' } }) as const
// After this long without input the camera may follow the agent's edits.
const IDLE_MS = 3000

// A saved board that differs from what this client last showed carries
// someone else's edits: the user's own are on screen before they are saved,
// and saving is what moves the version on. Those items
// flash; the camera follows them while the user is idle, otherwise a pill
// offers to.
export function useAgentFollow(local: CanvasView, inset: number, initialFit = true) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const store = useStoreApi<BoardNode, BoardEdge>()
  const touched = useRef(0)
  const settling = useRef(initialFit)
  const shown = useRef<CanvasView | null>(null)
  const [flash, setFlash] = useState<Record<string, number>>({})
  const [news, setNews] = useState<string[]>([])
  const [opening, setOpening] = useState(initialFit)
  const show = useCallback((ids: string[]) => {
    setNews([])
    requestAnimationFrame(() => void flow.fitView({ nodes: ids.map((id) => ({ id })), duration: 600, maxZoom: 1, ...fitPadding(inset) }))
  }, [flow, inset])

  useEffect(() => {
    const before = shown.current
    shown.current = local
    if (!before || local.version === before.version) return
    const changed = changedNodeIds(before, local)
    if (!changed.length) return
    settling.current = false
    const now = Date.now()
    setFlash((current) => ({ ...current, ...Object.fromEntries(changed.map((id) => [id, now])) }))
    const { width, height } = store.getState()
    const { x, y, zoom } = flow.getViewport()
    const visible = (id: string) => {
      const node = flow.getNode(id)
      if (!node) return false
      const rect = rectOfNode(node)
      return rect.x * zoom + x >= inset && rect.y * zoom + y >= 0 && (rect.x + rect.width) * zoom + x <= width && (rect.y + rect.height) * zoom + y <= height
    }
    if (changed.every(visible)) return
    if (now - touched.current > IDLE_MS) show(changed)
    else setNews(changed)
  }, [local, flow, store, show, inset])

  // Items take their size as their content loads, so the opening view fits
  // them again each time, until the user or the agent moves on. It fits in
  // the pass that measures them, and the board stays `opening`, its items
  // unseen, until the first one, so they are never painted unfitted.
  const settle = useCallback(() => {
    setOpening(false)
    if (!settling.current) return
    const drawn = local.nodes.map((item) => item.id).filter((id) => flow.getInternalNode(id)?.measured.height)
    if (!drawn.length) return
    const { width, height, minZoom } = store.getState()
    void flow.setViewport(getViewportForBounds(flow.getNodesBounds(drawn), width, height, minZoom, 1, fitPadding(inset).padding))
  }, [flow, store, local, inset])
  const touch = useCallback(() => {
    touched.current = Date.now()
    settling.current = false
  }, [])
  return { flash, news, show, touch, settle, opening }
}
