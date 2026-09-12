export type VoiceStatus = {
  phase: 'off' | 'connecting' | 'listening' | 'ending' | 'error'
  muted: boolean
  speakerMuted: boolean
  working: boolean
  error: string
}

export type VoiceOverlayState = VoiceStatus & {
  sessionId: string
  docked: boolean
  level: number
  dark: boolean
  reducedMotion: boolean
}

export type VoiceCommand = 'mute' | 'muteSpeaker' | 'exit' | 'reconnect' | 'return'

export interface VoiceOverlayAPI {
  drag: (point: { x: number; y: number } | null) => void
  publish: (state: VoiceOverlayState | null) => void
  subscribe: (handler: (state: VoiceOverlayState | null) => void) => () => void
  command: (command: VoiceCommand) => void
  onCommand: (handler: (command: VoiceCommand) => void) => () => void
}
