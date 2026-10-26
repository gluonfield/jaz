export interface VoiceSignaling {
  start(sessionId: string, sdp: string, context: string): Promise<void>
  append(text: string, speak?: boolean, delegationId?: string, sourceSeq?: number): void
  end(): void
  dispose(): void
}
