import { openEventStream } from '@/lib/api/eventStream'
import type { SessionEvent } from './types'

export function openSessionEvents(
  sessionId: string,
  afterSeq: number,
  onEvent: (event: SessionEvent) => void,
  onConnection?: (connected: boolean) => void,
): () => void {
  const suffix = afterSeq > 0 ? `?after_seq=${afterSeq}` : ''

  const handle = (ev: MessageEvent) => {
    try {
      onEvent(JSON.parse(ev.data as string) as SessionEvent)
    } catch {
      // skip malformed frames
    }
  }

  return openEventStream(`/v1/sessions/${sessionId}/events${suffix}`, { message: handle }, onConnection)
}
