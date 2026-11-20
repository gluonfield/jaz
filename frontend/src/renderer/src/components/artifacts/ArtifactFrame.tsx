import { LoaderCircle } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { artifactErrorForDisplay, buildArtifactDocument, type AgentState, type ArtifactInput } from '@/lib/artifacts'
import { useArtifactThemeCSS } from '@/lib/hooks/useArtifactThemeCSS'

interface ArtifactMessage {
  type?: string
  height?: number
  message?: string
  text?: string
  files?: unknown[]
  href?: string
  name?: string
  title?: string
  state?: unknown
}

const MIN_HEIGHT = 180

function clampHeight(height: number): number {
  if (!Number.isFinite(height)) return MIN_HEIGHT
  return Math.max(MIN_HEIGHT, Math.ceil(height))
}

export function ArtifactFrame({ input, data, agentState, viewState, pending = false, executionError = '', onSendPrompt, onOpenLink, onOpenView, onViewState }: {
  input: ArtifactInput
  data?: unknown
  agentState?: AgentState
  viewState?: unknown
  pending?: boolean
  executionError?: string
  onSendPrompt?: (text: string, files: File[]) => void
  onOpenLink?: (href: string) => boolean
  onOpenView?: (name: string, title: string) => void
  onViewState?: (state: unknown) => void
}) {
  const theme = useArtifactThemeCSS()
  const frameRef = useRef<HTMLIFrameElement>(null)
  const savedViewState = useRef(viewState)
  const [height, setHeight] = useState(280)
  const [error, setError] = useState('')
  const [built] = useState({ data, agentState })
  const doc = useMemo(() => {
    if (executionError) return ''
    return buildArtifactDocument(input, theme, { ...built, viewState: savedViewState.current })
  }, [built, executionError, input, theme])
  const sendData = useCallback(() => {
    const frame = frameRef.current?.contentWindow
    if (data !== built.data) frame?.postMessage({ type: 'jaz:canvas-data', data }, '*')
    if (agentState) frame?.postMessage({ type: 'jaz:artifact-agent-state', state: agentState }, '*')
  }, [built, data, agentState])
  useEffect(sendData, [sendData])

  useEffect(() => {
    setHeight(280)
    setError('')
  }, [doc])

  useEffect(() => {
    const onMessage = (event: MessageEvent<ArtifactMessage>) => {
      if (event.source !== frameRef.current?.contentWindow) return
      const message = event.data
      if (message?.type === 'jaz:artifact-height') setHeight(clampHeight(message.height ?? 0))
      if (message?.type === 'jaz:artifact-error') {
        const error = artifactErrorForDisplay(message.message ?? 'Artifact error')
        if (error) setError(error)
      }
      if (message?.type === 'jaz:artifact-link' && message.href && !onOpenLink?.(message.href)) window.open(message.href, '_blank', 'noopener,noreferrer')
      if (message?.type === 'jaz:artifact-open-view' && typeof message.name === 'string' && typeof message.title === 'string') onOpenView?.(message.name, message.title)
      if (message?.type === 'jaz:artifact-view-state' && onViewState) {
        savedViewState.current = message.state ?? null
        onViewState(savedViewState.current)
      }
      if (message?.type === 'jaz:artifact-send-prompt' && message.text?.trim()) onSendPrompt?.(message.text.trim(), message.files?.filter((file): file is File => file instanceof File) ?? [])
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [onOpenLink, onSendPrompt, onOpenView, onViewState])

  const displayError = executionError || error

  return (
    <div className="w-full">
      {displayError ? (
        <pre className="m-0 whitespace-pre-wrap bg-danger-soft px-3 py-2 font-mono text-[12px] text-danger">
          {displayError}
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
          onLoad={sendData}
          style={{ height }}
          className="block w-full border-0 bg-transparent"
        />
      )}
    </div>
  )
}
