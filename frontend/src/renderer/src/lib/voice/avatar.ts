import type { VoiceStatus } from '@shared/voice'

export type VoiceAvatarState = 'connecting' | 'listening' | 'thinking' | 'working' | 'speaking' | 'muted' | 'error'

export class VoiceAvatarActivity {
  private inputUntil = 0
  private outputUntil = 0

  sample(voice: VoiceStatus, input: number, output: number, now: number): VoiceAvatarState {
    if (voice.phase !== 'listening') {
      this.inputUntil = 0
      this.outputUntil = 0
      return voice.phase === 'connecting' ? 'connecting' : voice.phase === 'error' ? 'error' : 'muted'
    }
    if (output > 0.035) {
      this.outputUntil = now + 320
    }
    if (!voice.muted && input > 0.08) {
      this.inputUntil = now + 400
    }
    if (now < this.outputUntil) {
      return 'speaking'
    }
    if (!voice.muted && now < this.inputUntil) {
      return 'listening'
    }
    return voice.activity ?? (voice.muted ? 'muted' : 'listening')
  }
}
