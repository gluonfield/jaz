export type UpdateStatus =
  | { state: 'idle' }
  | { state: 'available'; version?: string }
  | { state: 'downloading'; version?: string; percent: number }
  | { state: 'downloaded'; version?: string }
  | { state: 'error'; message: string }

