import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useToast } from '@/components/ui/toast'
import { getCanvas, patchCanvas, type CanvasDocument, type CanvasPagePatch, type CanvasPatch } from '@/lib/api/canvas'
import { keys } from '@/lib/query/keys'
import { applyPagePatch, invertPatch } from './board'

// The server's board plus the user's edits still in flight. The board is polled
// while the agent works and read once more when its turn ends. Undo and redo
// replay the inverse of the user's own patches, so the agent's edits stay.
export function useCanvasSync(sessionId: string, working: boolean) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const canvas = useQuery({
    queryKey: keys.sessionCanvas(sessionId),
    queryFn: () => getCanvas(sessionId, queryClient.getQueryData<CanvasDocument>(keys.sessionCanvas(sessionId))),
    refetchInterval: working ? 1000 : false,
  })
  const { refetch } = canvas
  const wasWorking = useRef(working)
  useEffect(() => {
    if (wasWorking.current && !working) void refetch()
    wasWorking.current = working
  }, [working, refetch])

  const view = canvas.data
  const [pending, setPending] = useState<CanvasPagePatch[]>([])
  const local = useMemo(() => (view ? pending.reduce(applyPagePatch, view) : undefined), [view, pending])
  // Edits made in one event must see each other before React renders.
  const latest = useRef(local)
  useLayoutEffect(() => {
    latest.current = local
  }, [local])
  const history = useRef(new Map<string, Record<'undo' | 'redo', CanvasPatch[]>>())

  const send = useCallback((change: CanvasPagePatch) => {
    const document = latest.current!
    const page = document.pages.find((page) => page.id === change.page)!
    const inverse = invertPatch(page, change)
    latest.current = applyPagePatch(document, change)
    setPending((current) => [...current, change])
    patchCanvas(sessionId, change)
      .catch((error: Error) => toast(`Couldn't save the canvas: ${error.message}`, 'danger'))
      .then(() => refetch())
      .finally(() => setPending((current) => current.filter((item) => item !== change)))
    return inverse
  }, [sessionId, refetch, toast])
  const commit = useCallback((page: string, change: CanvasPatch) => {
    const stack = history.current.get(page) ?? { undo: [], redo: [] }
    stack.undo.push(send({ ...change, page }))
    stack.redo = []
    history.current.set(page, stack)
  }, [send])
  const replay = useCallback((page: string, from: 'undo' | 'redo') => {
    const stack = history.current.get(page)
    const change = stack?.[from].pop()
    if (stack && change) {
      stack[from === 'undo' ? 'redo' : 'undo'].push(send({ ...change, page }))
    }
  }, [send])
  const createPage = useCallback(async (page: string) => {
    await patchCanvas(sessionId, { page, create: true })
    await refetch()
  }, [sessionId, refetch])
  const renamePage = useCallback((page: string, name: string) => {
    send({ page, name })
  }, [send])
  return { view, local, commit, replay, createPage, renamePage, error: canvas.error }
}
