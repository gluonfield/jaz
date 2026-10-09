export type MicrophoneAccess = 'not-determined' | 'granted' | 'denied' | 'restricted' | 'unknown'

export interface MicrophoneAPI {
  status(): Promise<MicrophoneAccess>
  allow(): Promise<boolean>
}
