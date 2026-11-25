import { memo, useCallback, useMemo } from 'react'
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
  return <ArtifactFrame input={input} data={data} agentState={agentState} viewState={viewState} pluginView={pluginView} assetOrigin={assetOrigin} pending={pending} executionError={artifactErrorForDisplay(toolError(result))} onSendPrompt={onSendPrompt} onOpenLink={openLink} onOpenView={onOpenView} onViewState={onViewState} onResolveAsset={onResolveAsset} />
})
