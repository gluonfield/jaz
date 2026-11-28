export interface EndpointConfig {
  speechLevel: number
  silenceLevel: number
  silenceMs: number
  noSpeechMs: number
  maxDurationMs: number
}

export const DEFAULT_ENDPOINT_CONFIG: EndpointConfig = {
  speechLevel: 0.025,
  silenceLevel: 0.015,
  silenceMs: 1600,
  noSpeechMs: 7000,
  maxDurationMs: 60_000,
}

export type EndpointDecision = 'continue' | 'stop' | 'no_speech'

export class DictationEndpoint {
  private startedAt: number
  private heardSpeech = false
  private lastActivityAt: number
  private lastText = ''

  constructor(
    startedAt: number,
    private readonly config: EndpointConfig = DEFAULT_ENDPOINT_CONFIG,
  ) {
    this.startedAt = startedAt
    this.lastActivityAt = startedAt
  }

  onLevel(level: number, now: number): EndpointDecision {
    if (!this.heardSpeech && level >= this.config.speechLevel) {
      this.heardSpeech = true
      this.lastActivityAt = now
    } else if (this.heardSpeech && level >= this.config.silenceLevel) {
      this.lastActivityAt = now
    }
    return this.tick(now)
  }

  onTranscript(text: string, now: number): EndpointDecision {
    const trimmed = text.trim()
    if (trimmed && trimmed !== this.lastText) {
      this.lastText = trimmed
      this.heardSpeech = true
      this.lastActivityAt = now
    }
    return this.tick(now)
  }

  tick(now: number): EndpointDecision {
    if (now - this.startedAt >= this.config.maxDurationMs) {
      return this.heardSpeech ? 'stop' : 'no_speech'
    }
    if (!this.heardSpeech) {
      return now - this.startedAt >= this.config.noSpeechMs ? 'no_speech' : 'continue'
    }
    return now - this.lastActivityAt >= this.config.silenceMs ? 'stop' : 'continue'
  }
}
