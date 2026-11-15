import { memo, useMemo } from 'react'
import { ArtifactFrame } from '@/components/artifacts/ArtifactFrame'
import type { ArtifactEvent } from '@/lib/api/types'
import { artifactErrorForDisplay, artifactInputFromEvent, parseArtifactToolArgs } from '@/lib/artifacts'

function toolError(result?: string): string {
  if (!result) return ''
  try {
    const parsed = JSON.parse(result) as { status?: string; error?: string }
    return parsed.status === 'error' ? parsed.error ?? 'Artifact failed.' : ''
  } catch {
    return ''
  }
}

export const ArtifactBlock = memo(function ArtifactBlock({ artifact, args, result, pending = false, data = artifact?.data, onSendPrompt, onOpenFile }: {
  artifact?: ArtifactEvent
  args?: string
  result?: string
  pending?: boolean
  data?: unknown
  onSendPrompt?: (text: string) => void
  onOpenFile?: (href: string) => boolean
}) {
  const input = useMemo(
    () => (artifact ? artifactInputFromEvent(artifact) : parseArtifactToolArgs(args)),
    [artifact, args],
  )
  if (!input) {
    return <div className="rounded-card border border-border bg-surface px-3 py-2 text-sm text-ink-2">Could not render artifact: invalid visualise_show_widget input.</div>
  }
  return <ArtifactFrame input={input} data={data} pending={pending} executionError={artifactErrorForDisplay(toolError(result))} onSendPrompt={onSendPrompt} onOpenLink={onOpenFile} />
})
