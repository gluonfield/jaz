import { apiFetch } from './client'
import {
  readSessionMessageStream,
  type AgentStreamEvent,
  type SessionMessageStream,
} from './streamProtocol'
import type { MessageContextInput } from '@/lib/messageContext'
import { telemetry } from '@/lib/telemetry'

export type { AgentStreamEvent } from './streamProtocol'

// POST + SSE response: EventSource can't send a body, so parse the stream
// off fetch. Frames are `event: <type>\ndata: <json>\n\n`.
export function streamSessionMessage({
  sessionId,
  message,
  contexts = [],
  attachmentIds = [],
  planRequested = false,
  goalRequested = false,
  voice = false,
  signal,
  onEvent,
}: {
  sessionId: string
  message: string
  contexts?: MessageContextInput[]
  attachmentIds?: string[]
  planRequested?: boolean
  goalRequested?: boolean
  voice?: boolean
  signal: AbortSignal
  onEvent: (event: AgentStreamEvent) => void
}): SessionMessageStream {
  const response = apiFetch(`/v1/sessions/${sessionId}/messages:stream`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      message,
      contexts,
      attachment_ids: attachmentIds,
      plan_requested: planRequested,
      goal_requested: goalRequested,
      voice,
    }),
    signal,
  }).then((res) => {
    if (res.ok && res.body) {
      telemetry.messageSent({
        queued: false,
        voice,
        planRequested,
        goalRequested,
        attachmentCount: attachmentIds.length,
      })
    }
    return res
  })
  return readSessionMessageStream(response, onEvent)
}
