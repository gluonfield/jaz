import { PanelRight, Share2 } from 'lucide-react'
import { memo, use, useCallback, useMemo, useState } from 'react'
import { PublishAppModal } from '@/components/apps/PublishAppModal'
import { ArtifactFrame } from '@/components/artifacts/ArtifactFrame'
import { IconButton } from '@/components/ui/IconButton'
import { ArtifactPanelContext } from '@/lib/artifactPanel'
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
  const panel = use(ArtifactPanelContext)
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
        <div className="flex h-8 items-center gap-0.5">
          <span className="min-w-0 flex-1 truncate text-[12.5px] text-ink-3">{input.title}</span>
          <IconButton size="sm" aria-label="Publish as app" title="Publish as app" onClick={() => setPublishOpen(true)}>
            <Share2 size={14} />
          </IconButton>
          {panel ? (
            <IconButton size="sm" aria-label="Open in side panel" title="Open in side panel" onClick={() => panel.openArtifact(input)}>
              <PanelRight size={14} />
            </IconButton>
          ) : null}
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
          onPublished={panel?.openPublishedApp}
        />
      ) : null}
    </div>
  )
})
