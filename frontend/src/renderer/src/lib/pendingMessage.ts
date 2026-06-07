// Hands the first message from the New-session page to the session view.
// The session page waits until its detail query has loaded before taking it.
const pending = new Map<string, string>()

export function setPendingMessage(sessionId: string, message: string): void {
  pending.set(sessionId, message)
}

export function takePendingMessage(sessionId: string): string | undefined {
  const message = pending.get(sessionId)
  pending.delete(sessionId)
  return message
}
