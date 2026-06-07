import { apiBaseUrl } from './client'
import type { ToolCallJSON } from './types'

// agent.StreamEvent on the wire (backend/internal/agent/agent.go).
export interface AgentStreamEvent {
  type: 'delta' | 'reasoning' | 'tool_call' | 'tool_result' | 'error' | 'done' | (string & {})
  delta?: string
  reasoning?: string
  tool_call?: ToolCallJSON
  tool_name?: string
  result?: string
  error?: string
  at?: string
}

// POST + SSE response: EventSource can't send a body, so parse the stream
// off fetch. Frames are `event: <type>\ndata: <json>\n\n`.
export async function streamSessionMessage({
  sessionId,
  message,
  planRequested = false,
  voice = false,
  signal,
  onEvent,
}: {
  sessionId: string
  message: string
  planRequested?: boolean
  voice?: boolean
  signal: AbortSignal
  onEvent: (event: AgentStreamEvent) => void
}): Promise<void> {
  const res = await fetch(`${apiBaseUrl()}/v1/sessions/${sessionId}/messages:stream`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message, plan_requested: planRequested, voice }),
    signal,
  })
  if (!res.ok || !res.body) {
    let detail = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) detail = body.error
    } catch {
      // keep status text
    }
    throw new Error(detail)
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const handleFrame = (frame: string) => {
    for (const line of frame.split('\n')) {
      if (!line.startsWith('data: ')) continue
      try {
        onEvent(JSON.parse(line.slice(6)) as AgentStreamEvent)
      } catch {
        // skip malformed frames
      }
    }
  }

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let sep = buffer.indexOf('\n\n')
    while (sep !== -1) {
      handleFrame(buffer.slice(0, sep))
      buffer = buffer.slice(sep + 2)
      sep = buffer.indexOf('\n\n')
    }
  }
  if (buffer.trim()) handleFrame(buffer)
}
