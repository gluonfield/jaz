import { useQuery } from '@tanstack/react-query'
import { use, useCallback, useMemo, useRef, useState } from 'react'
import { ArtifactBlock } from '@/components/session/ArtifactBlock'
import { openPluginView, pluginViewStateQuery, savePluginViewState } from '@/lib/api/plugins'
import { sessionFileQuery } from '@/lib/api/sessions'
import { useRefetchAfterWork } from '@/lib/hooks/useRefetchAfterWork'
import { MCPAppThreadContext } from '@/lib/mcpAppThread'
import type { PanelView, PanelPresentation } from '@/lib/sidePanelTabs'
import { resolveFileLink, type FileReference } from '@shared/fileReader'

function parse(content?: string): unknown {
  try {
    return content ? JSON.parse(content) : undefined
  } catch {
    return undefined
  }
}

// A plugin's view in the side panel draws a JSON file in the thread's folder
// and follows it while the agent works, as a document does.
export function ViewPanel({ sessionId, cloudRunId, view, visible, working, asking, onOpenFile, onOpenView }: {
  sessionId: string
  cloudRunId?: string
  view: PanelView
  visible: boolean
  working: boolean
  asking: boolean
  onOpenFile: (file: FileReference) => boolean
  onOpenView: (view: PanelPresentation) => void
}) {
  const thread = use(MCPAppThreadContext)
  const [error, setError] = useState('')
  const file = useQuery({ ...sessionFileQuery(sessionId, view.file, cloudRunId), enabled: visible, staleTime: 0, refetchInterval: working ? 2000 : false })
  useRefetchAfterWork(working, file.refetch)
  const { reference } = view
  const state = useQuery({ ...pluginViewStateQuery(sessionId, { reference: reference ?? '', file: view.file }), enabled: visible && Boolean(reference) })
  const unsaved = useRef<{ state: unknown } | null>(null)
  const saving = useRef(false)
  // Saves run one at a time and only the newest waits, so a view may save on every keystroke.
  const saveState = useCallback(async (state: unknown) => {
    if (!reference) return
    unsaved.current = { state }
    if (saving.current) return
    saving.current = true
    while (unsaved.current) {
      const next = unsaved.current.state
      unsaved.current = null
      await savePluginViewState(sessionId, { reference, file: view.file }, next).then(() => setError(''), () => setError('Could not save your progress in this view.'))
    }
    saving.current = false
  }, [sessionId, reference, view.file])
  const content = file.data?.content
  const data = useMemo(() => parse(content), [content])
  const path = file.data?.path
  const openFile = useCallback((href: string) => {
    const linked = resolveFileLink(href, path)
    return linked ? onOpenFile(linked) : false
  }, [path, onOpenFile])
  const openSibling = useCallback(async (name: string, title: string) => {
    if (!view.reference) return
    try {
      const target = await openPluginView(sessionId, { reference: view.reference, file: view.file, name, title })
      onOpenView({ kind: 'plugin-view', view: target })
      setError('')
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not open view.')
    }
  }, [sessionId, view.reference, view.file, onOpenView])
  if (!view.widget_code) {
    return <p className="px-5 py-6 text-sm text-ink-3">Plugin view unavailable.</p>
  }
  if (data === undefined || state.isLoading) {
    return <p className="px-5 py-6 text-sm text-ink-3">{file.isPending || state.isLoading ? 'Loading…' : `Nothing to show yet: ${view.file} is missing or is not JSON.`}</p>
  }
  return (
    <div className="h-full overflow-y-auto">
      {error ? <p role="alert" className="px-3 py-2 text-sm text-danger">{error}</p> : null}
      <ArtifactBlock artifact={view} data={data} agentState={asking ? 'asking' : working ? 'working' : 'idle'} viewState={state.isSuccess ? state.data ?? null : undefined} onViewState={saveState} onSendPrompt={thread?.send} onOpenFile={openFile} onOpenView={view.reference ? openSibling : undefined} />
    </div>
  )
}
