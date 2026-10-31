import { useCallback, useMemo, useRef, useState } from 'react'
import type { CanvasPage } from '@/lib/api/canvas'
import type { ComposerContext } from '@/lib/sendMessage'

export interface CanvasPick {
  page: string
  node: string
  label: string
}

interface DraftContexts {
  contexts: ComposerContext[]
  removeContext: (id: string) => void
  replaceContexts: (contexts: ComposerContext[]) => void
}

const contextId = (page: string, node?: string) => `canvas:${JSON.stringify([page, node])}`

// The canvas selection rides along as composer context. It lives in the board,
// never in the stored draft: sending clears it, a removed chip deselects.
export function useCanvasContexts(draft: DraftContexts) {
  const [page, setPage] = useState<Pick<CanvasPage, 'id' | 'name'>>()
  const [picks, setPicks] = useState<CanvasPick[]>([])
  const controller = useRef<((picks: CanvasPick[]) => void) | undefined>(undefined)
  const bindSelection = useCallback((select: (picks: CanvasPick[]) => void) => {
    controller.current = select
    return () => {
      if (controller.current === select) controller.current = undefined
    }
  }, [])
  const onPage = useCallback((next: Pick<CanvasPage, 'id' | 'name'>) => {
    setPage((current) => current?.id === next.id && current.name === next.name ? current : next)
    setPicks((current) => current.every((pick) => pick.page === next.id) ? current : [])
  }, [])
  const onSelect = useCallback((next: CanvasPick[]) => {
    setPicks((current) => (current.length === next.length && current.every((pick, i) => pick.page === next[i].page && pick.node === next[i].node && pick.label === next[i].label) ? current : next))
  }, [])
  const contexts = useMemo<ComposerContext[]>(() => [
    ...draft.contexts,
    ...(picks.length
      ? picks.map((pick) => ({ id: contextId(pick.page, pick.node), type: 'canvas' as const, ...pick }))
      : page ? [{ id: contextId(page.id), type: 'canvas' as const, page: page.id, label: page.name }] : []),
  ], [draft.contexts, page, picks])
  const { removeContext, replaceContexts } = draft
  const remove = useCallback((id: string) => {
    if (id.startsWith('canvas:')) controller.current?.(picks.filter((pick) => contextId(pick.page, pick.node) !== id))
    else removeContext(id)
  }, [removeContext, picks])
  const replace = useCallback((next: ComposerContext[]) => {
    replaceContexts(next.filter((context) => context.type !== 'canvas'))
    controller.current?.(next.flatMap((context) => (context.type === 'canvas' && context.node ? [{ page: context.page, node: context.node, label: context.label }] : [])))
  }, [replaceContexts])
  const placeholder = ['Ask about the canvas', 'Ask about this', 'Ask about these'][Math.min(picks.length, 2)]
  return { bindSelection, onSelect, onPage, contexts, removeContext: remove, replaceContexts: replace, placeholder }
}
