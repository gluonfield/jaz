export interface VoiceDiagnostic {
  id: string
  call_id: string
  stage: string
  request_id?: string
  user_turn_id?: string
  event_id?: string
  source_seq?: number
  at: string
  elapsed_ms: number
  provider_start_ms?: number
  provider_end_ms?: number
  detail?: string
}

export type VoiceTrace = (stage: string, detail?: Pick<VoiceDiagnostic, 'request_id' | 'user_turn_id' | 'event_id' | 'source_seq' | 'provider_start_ms' | 'provider_end_ms' | 'detail'>) => void
