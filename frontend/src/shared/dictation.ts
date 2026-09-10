export type DictationPhase = 'starting' | 'downloading' | 'recording' | 'transcribing'

export type DictationEvent =
  | { type: 'status'; phase: DictationPhase }
  | { type: 'level'; level: number } // Linear RMS amplitude, from 0 to 1.
  | { type: 'result'; text: string }
  | { type: 'complete'; text: string }
  | { type: 'error'; message: string }

export type DictationAvailability = { available: true } | { available: false; reason: string }

export interface DictationAPI {
  availability: () => Promise<DictationAvailability>
  start: (id: string) => Promise<void>
  stop: (id: string) => Promise<void>
  cancel: (id: string) => Promise<void>
  onEvent: (handler: (id: string, event: DictationEvent) => void) => () => void
}

export function parseDictationEvent(value: unknown): DictationEvent {
  if (typeof value === 'object' && value !== null && 'type' in value) {
    if (value.type === 'status' && 'phase' in value && (
      value.phase === 'starting' || value.phase === 'downloading' ||
      value.phase === 'recording' || value.phase === 'transcribing'
    )) {
      return { type: 'status', phase: value.phase }
    }
    if (value.type === 'level' && 'level' in value && typeof value.level === 'number' && Number.isFinite(value.level)) {
      return { type: 'level', level: Math.max(0, Math.min(1, value.level)) }
    }
    if ((value.type === 'result' || value.type === 'complete') && 'text' in value && typeof value.text === 'string') {
      return { type: value.type, text: value.text }
    }
    if (value.type === 'error' && 'message' in value && typeof value.message === 'string') {
      return { type: 'error', message: value.message }
    }
  }
  throw new Error('Invalid native dictation event')
}
