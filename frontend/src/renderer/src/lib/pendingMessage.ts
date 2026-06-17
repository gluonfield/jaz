// Hands the first message from the New-session page to the session view.
// The session page waits until its detail query has loaded before taking it.
export interface PendingMessage {
  text: string
  planRequested?: boolean
  files?: File[]
}

const pending = new Map<string, PendingMessage>()

export function setPendingMessage(sessionId: string, message: string | PendingMessage): void {
  if (typeof message === 'string') {
    pending.set(sessionId, { text: message })
    return
  }
  pending.set(sessionId, message)
}

export function takePendingMessage(sessionId: string): PendingMessage | undefined {
  const message = pending.get(sessionId)
  pending.delete(sessionId)
  return message
}
