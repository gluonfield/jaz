import { Share2 } from 'lucide-react'
import { memo, useCallback, useMemo, useState } from 'react'
import { PublishAppModal } from '@/components/apps/PublishAppModal'
import { ArtifactFrame } from '@/components/artifacts/ArtifactFrame'
import { useFileLinks } from '@/lib/fileLinks'
import { resolveFileLink } from '@shared/fileReader'
import type { ArtifactEvent } from '@/lib/api/types'
import { artifactErrorForDisplay, artifactInputFromEvent, parseArtifactToolArgs, type AgentState, type ResolvedAsset } from '@/lib/artifacts'

function toolError(result?: string): string {
  if (!result) return ''
  try {
    const parsed = JSON.parse(result) as { status?: string; error?: string }
    return parsed.status === 'error' ? parsed.error ?? 'Artifact failed.' : ''
  } catch {
    return ''
  }
}

export const ArtifactBlock = memo(function ArtifactBlock({ artifact, args, result, pending = false, data = artifact?.data, agentState, viewState, pluginView = false, assetOrigin, onSendPrompt, onOpenFile, onOpenView, onViewState, onResolveAsset }: {
  artifact?: ArtifactEvent
  args?: string
  result?: string
  pending?: boolean
  data?: unknown
  agentState?: AgentState
  viewState?: unknown
  pluginView?: boolean
  assetOrigin?: string
  onSendPrompt?: (text: string) => void
  onOpenFile?: (href: string) => boolean
  onOpenView?: (name: string, title: string) => void
  onViewState?: (state: unknown) => void
  onResolveAsset?: (path: string) => Promise<ResolvedAsset>
}) {
  const files = useFileLinks()
  const [publishOpen, setPublishOpen] = useState(false)
  const openLink = useCallback((href: string) => {
    if (onOpenFile) return onOpenFile(href)
    if (!files) return false
    const file = resolveFileLink(href, files.documentPath)
    if (!file) return false
    files.onOpen(file)
    return true
  }, [files, onOpenFile])
  const input = useMemo(
    () => (artifact ? artifactInputFromEvent(artifact) : parseArtifactToolArgs(args)),
    [artifact, args],
  )
  if (!input) {
    return <div className="rounded-card border border-border bg-surface px-3 py-2 text-sm text-ink-2">Could not render artifact: invalid visualise_show_widget input.</div>
  }
  const execError = artifactErrorForDisplay(toolError(result))
  const canPublish = !pending && !pluginView && !execError
  return (
    <div className="group/artifact relative w-full">
      {canPublish ? (
        <div className="flex items-center justify-between gap-2 border-b border-border/60 bg-surface/70 px-3 py-1.5">
          <span className="truncate text-[12px] font-medium text-ink-2">{input.title}</span>
          <button
            type="button"
            onClick={() => setPublishOpen(true)}
            className="inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-control px-2 py-0.5 text-[11.5px] font-medium text-ink-2 transition-colors hover:bg-surface-2 hover:text-ink"
          >
            <Share2 size={12} aria-hidden />
            Publish
          </button>
        </div>
      ) : null}
      <ArtifactFrame input={input} data={data} agentState={agentState} viewState={viewState} pluginView={pluginView} assetOrigin={assetOrigin} pending={pending} executionError={execError} onSendPrompt={onSendPrompt} onOpenLink={openLink} onOpenView={onOpenView} onViewState={onViewState} onResolveAsset={onResolveAsset} />
      {publishOpen ? (
        <PublishAppModal
          open
          onClose={() => setPublishOpen(false)}
          initialTitle={input.title}
          kind="artifact"
          html={input.code}
        />
      ) : null}
    </div>
  )
})
