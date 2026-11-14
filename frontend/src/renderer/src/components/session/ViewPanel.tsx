import { useQuery } from '@tanstack/react-query'
import { use, useCallback, useMemo } from 'react'
import { ArtifactBlock } from '@/components/session/ArtifactBlock'
import { sessionFileQuery } from '@/lib/api/sessions'
import type { ArtifactEvent } from '@/lib/api/types'
import { useRefetchAfterWork } from '@/lib/hooks/useRefetchAfterWork'
import { MCPAppThreadContext } from '@/lib/mcpAppThread'
import { isAbsoluteFilePath } from '@shared/fileReader'

function parse(content?: string): unknown {
  try {
    return content ? JSON.parse(content) : undefined
  } catch {
    return undefined
  }
}

// A plugin's view in the side panel draws a JSON file in the thread's folder
// and follows it while the agent works, as a document does.
export function ViewPanel({ sessionId, view, visible, working, onOpenFile }: {
  sessionId: string
  view: ArtifactEvent & { file: string }
  visible: boolean
  working: boolean
  onOpenFile: (path: string) => void
}) {
  const thread = use(MCPAppThreadContext)
  const file = useQuery({ ...sessionFileQuery(sessionId, view.file), enabled: visible, refetchInterval: working ? 2000 : false })
  useRefetchAfterWork(working, file.refetch)
  const content = file.data?.content
  const data = useMemo(() => parse(content), [content])
  const folder = file.data?.path.replace(/[\\/][^\\/]*$/, '')
  const openFile = useCallback((path: string) => onOpenFile(isAbsoluteFilePath(path) ? path : `${folder}/${path}`), [folder, onOpenFile])
  if (data === undefined) {
    return <p className="px-5 py-6 text-sm text-ink-3">{file.isPending ? 'Loading…' : `Nothing to show yet: ${view.file} is missing or is not JSON.`}</p>
  }
  return (
    <div className="h-full overflow-y-auto">
      <ArtifactBlock artifact={view} data={data} onSendPrompt={thread?.send} onOpenFile={openFile} />
    </div>
  )
}
