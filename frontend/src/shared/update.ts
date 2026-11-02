export type UpdateStatus =
  | { state: 'idle' }
  | { state: 'checking' }
  | { state: 'available'; version?: string }
  | { state: 'downloading'; version?: string; percent: number }
  | { state: 'downloaded'; version?: string; error?: string }
  | { state: 'installing'; version?: string }
  | { state: 'error'; message: string }
