export interface VoiceSignaling {
  start(sessionId: string, sdp: string, context: string): Promise<void>
  append(text: string, speak?: boolean, delegationId?: string, sourceSeq?: number): void
  end(): void
  dispose(): void
}

export function voiceAnswerSDP(value: unknown): string {
  if (typeof value !== 'string' || !/^v=0\r?\n/.test(value)) {
    throw new Error('The voice provider returned an invalid connection answer. Please try again.')
  }
  return value
}
