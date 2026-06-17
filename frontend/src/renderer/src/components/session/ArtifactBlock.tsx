import { LoaderCircle } from 'lucide-react'
import { memo, useEffect, useMemo, useRef, useState } from 'react'
import type { ArtifactEvent } from '@/lib/api/types'
import {
  artifactInputFromEvent,
  buildArtifactDocument,
  buildArtifactThemeCSS,
  parseArtifactToolArgs,
} from '@/lib/artifacts'

interface ArtifactMessage {
  type?: string
  height?: number
  message?: string
  text?: string
  href?: string
}

const MIN_HEIGHT = 180
const MAX_HEIGHT = 900

function clampHeight(height: number): number {
  if (!Number.isFinite(height)) return MIN_HEIGHT
  return Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, Math.ceil(height)))
}

function toolError(result?: string): string {
  if (!result) return ''
  try {
    const parsed = JSON.parse(result) as { status?: string; error?: string }
    return parsed.status === 'error' ? parsed.error ?? 'Artifact failed.' : ''
  } catch {
    return ''
  }
}

function useArtifactThemeCSS(): string {
  const [css, setCSS] = useState(() => buildArtifactThemeCSS())
  useEffect(() => {
    const update = () => setCSS(buildArtifactThemeCSS())
    update()
    const observer = new MutationObserver(update)
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] })
    return () => observer.disconnect()
  }, [])
  return css
}

export const ArtifactBlock = memo(function ArtifactBlock({
  artifact,
  args,
  result,
  pending = false,
  onSendPrompt,
}: {
  artifact?: ArtifactEvent
  args?: string
  result?: string
  pending?: boolean
  onSendPrompt?: (text: string) => void
}) {
  const input = useMemo(
    () => (artifact ? artifactInputFromEvent(artifact) : parseArtifactToolArgs(args)),
    [artifact, args],
  )
  const theme = useArtifactThemeCSS()
  const frameRef = useRef<HTMLIFrameElement>(null)
  const [height, setHeight] = useState(280)
  const [error, setError] = useState('')
  const executionError = toolError(result)
  const doc = useMemo(() => {
    if (!input || executionError) return ''
    return buildArtifactDocument(input, theme)
  }, [executionError, input, theme])

  useEffect(() => {
    setHeight(280)
    setError('')
  }, [doc])

  useEffect(() => {
    const onMessage = (event: MessageEvent<ArtifactMessage>) => {
      if (event.source !== frameRef.current?.contentWindow) return
      const message = event.data
      if (message?.type === 'jaz:artifact-height') setHeight(clampHeight(message.height ?? 0))
      if (message?.type === 'jaz:artifact-error') setError(message.message ?? 'Artifact error')
      if (message?.type === 'jaz:artifact-link' && message.href) window.open(message.href, '_blank', 'noopener,noreferrer')
      if (message?.type === 'jaz:artifact-send-prompt' && message.text?.trim()) onSendPrompt?.(message.text.trim())
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [onSendPrompt])

  if (!input) {
    return (
      <div className="rounded-card border border-border bg-surface px-3 py-2 text-sm text-ink-2">
        Could not render artifact: invalid visualise:show_widget input.
      </div>
    )
  }

  return (
    <div className="w-full">
      {executionError ? (
        <pre className="m-0 whitespace-pre-wrap bg-danger-soft px-3 py-2 font-mono text-[12px] text-danger">
          {executionError}
        </pre>
      ) : pending ? (
        <div className="flex min-h-10 items-center gap-1.5 px-3 py-2 text-[11px] text-ink-3">
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
          {input.loadingMessages[0] ?? 'Rendering'}
        </div>
      ) : (
        <iframe
          ref={frameRef}
          title={input.title}
          sandbox="allow-scripts allow-forms"
          referrerPolicy="no-referrer"
          srcDoc={doc}
          style={{ height }}
          className="block w-full border-0 bg-transparent"
        />
      )}
      {error && !executionError ? (
        <div className="border-t border-border bg-danger-soft px-3 py-1.5 font-mono text-[11px] text-danger">
          {error}
        </div>
      ) : null}
    </div>
  )
})
