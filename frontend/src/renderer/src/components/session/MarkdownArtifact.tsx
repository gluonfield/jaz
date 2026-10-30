import { createContext } from 'react'
import { ArtifactBlock } from '@/components/session/ArtifactBlock'

type ArtifactScope = {
  mode: 'source' | 'preview' | 'live'
  onSendPrompt?: (text: string) => void
}

export const MarkdownArtifactContext = createContext<ArtifactScope>({ mode: 'source' })

export function MarkdownArtifact({ title, code, mode, onSendPrompt }: ArtifactScope & { title: string; code: string }) {
  if (mode === 'preview') return <p className="text-sm text-ink-3">{title} · Interactive artifact</p>
  return <ArtifactBlock artifact={{ title, widget_code: code }} onSendPrompt={onSendPrompt} />
}
