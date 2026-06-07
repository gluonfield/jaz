import { apiBaseUrl } from './client'
import type { SessionEvent } from './types'

// Event types the backend names in its SSE frames (event: <type>).
// EventSource routes named events to addEventListener(type), NOT onmessage,
// so every type we care about must be registered explicitly. onmessage stays
// as a catch-all for unnamed frames.
const KNOWN_EVENT_TYPES = [
  'assistant',
  'user',
  'tool',
  'tool_result',
  'async',
  'error',
  'acp',
  'acp_message',
  'acp_thought',
  'acp_tool',
  'permission_request',
  'permission_response',
]

export function openSessionEvents(
  sessionId: string,
  onEvent: (event: SessionEvent) => void,
): () => void {
  const es = new EventSource(`${apiBaseUrl()}/v1/sessions/${sessionId}/events`)

  const handle = (ev: MessageEvent) => {
    try {
      onEvent(JSON.parse(ev.data as string) as SessionEvent)
    } catch {
      // skip malformed frames
    }
  }

  es.onmessage = handle
  for (const type of KNOWN_EVENT_TYPES) {
    es.addEventListener(type, handle)
  }

  return () => es.close()
}
