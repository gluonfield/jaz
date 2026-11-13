import { useCallback, useMemo, useRef, useState } from 'react'
import type { ComposerContext } from '@/lib/sendMessage'

type AppContext = Extract<ComposerContext, { type: 'app' }>

// What app views report of their state rides along as composer context, one
// chip per view, replaced by that view's next report. It never enters the
// stored draft: sending clears it, and removing the chip withholds it.
export function useAppContexts(draft: {
  contexts: ComposerContext[]
  removeContext: (id: string) => void
  replaceContexts: (contexts: ComposerContext[]) => void
}) {
  const [reported, setReported] = useState<AppContext[]>([])
  const report = useCallback((view: string, tool: string, text: string) => {
    const id = `app:${view}`
    setReported((current) => [...current.filter((context) => context.id !== id), ...(text ? [{ id, type: 'app' as const, tool, text }] : [])])
  }, [])
  const latest = useRef(reported)
  latest.current = reported
  // A send that bypasses the composer takes the reports with it.
  const take = useCallback(() => {
    const taken = latest.current
    if (taken.length) setReported([])
    return taken
  }, [])
  const contexts = useMemo(() => [...draft.contexts, ...reported], [draft.contexts, reported])
  const { removeContext, replaceContexts } = draft
  const remove = useCallback((id: string) => {
    if (id.startsWith('app:')) setReported((current) => current.filter((context) => context.id !== id))
    else removeContext(id)
  }, [removeContext])
  const replace = useCallback((next: ComposerContext[]) => {
    setReported(next.filter((context) => context.type === 'app'))
    replaceContexts(next.filter((context) => context.type !== 'app'))
  }, [replaceContexts])
  return { contexts, removeContext: remove, replaceContexts: replace, report, take }
}
