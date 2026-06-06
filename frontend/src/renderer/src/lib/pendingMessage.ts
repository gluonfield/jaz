// Hands the first message from the New-session page to the session view,
// which sends it on mount. A module map (not router state) so StrictMode's
// double-mount can't double-send: take() is destructive.
const pending = new Map<string, string>()

export function setPendingMessage(sessionId: string, message: string): void {
  pending.set(sessionId, message)
}

export function takePendingMessage(sessionId: string): string | undefined {
  const message = pending.get(sessionId)
  pending.delete(sessionId)
  return message
}
