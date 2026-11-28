import { LayoutDashboard } from 'lucide-react'
import { use, useMemo } from 'react'
import type { PresentationEvent } from '@/lib/api/types'
import { MapBlock } from '@/components/maps/MapBlock'
import { parsePlaceMap } from '@/lib/maps'
import { MCPAppThreadContext } from '@/lib/mcpAppThread'
import { ArtifactBlock } from './ArtifactBlock'

export function PresentedContent({ presentation, onSendPrompt }: {
  presentation: PresentationEvent
  onSendPrompt?: (text: string) => void
}) {
  const thread = use(MCPAppThreadContext)
  const map = useMemo(() => (presentation.kind === 'map' ? parsePlaceMap(presentation.map) : undefined), [presentation])
  if (presentation.kind === 'artifact') {
    return <ArtifactBlock artifact={presentation.artifact} onSendPrompt={onSendPrompt} />
  }
  if (presentation.kind === 'map') return map ? <MapBlock map={map} /> : null
  return (
    <button type="button" onClick={() => thread?.openView?.(presentation)} className="flex w-fit max-w-full cursor-pointer items-center gap-2 rounded-card border border-border bg-surface px-3 py-2 text-left text-sm text-ink transition-colors hover:bg-surface-2">
      <LayoutDashboard size={14} className="shrink-0 text-ink-3" aria-hidden />
      <span className="truncate">{presentation.kind === 'canvas' ? 'Canvas' : presentation.view.title}</span>
      <span className="shrink-0 text-ink-3">Open in side panel</span>
    </button>
  )
}
