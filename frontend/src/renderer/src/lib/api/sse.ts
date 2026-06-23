import { apiEventSourceUrl } from './client'
import type { SessionEvent } from './types'

export function openSessionEvents(
  sessionId: string,
  afterSeq: number,
  onEvent: (event: SessionEvent) => void,
): () => void {
  const suffix = afterSeq > 0 ? `?after_seq=${afterSeq}` : ''
  const es = new EventSource(apiEventSourceUrl(`/v1/sessions/${sessionId}/events${suffix}`))

  const handle = (ev: MessageEvent) => {
    try {
      onEvent(JSON.parse(ev.data as string) as SessionEvent)
    } catch {
      // skip malformed frames
    }
  }

  es.onmessage = handle

  return () => es.close()
}
