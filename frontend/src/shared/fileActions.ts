export const FILE_ACTIONS_CHANNEL = 'jaz:file-actions'
export const INSPECT_DROPPED_FILE_CHANNEL = 'jaz:files:inspect-drop'

export type DroppedPath =
  | { kind: 'file' }
  | { kind: 'directory'; path: string }
  | { kind: 'unavailable' }

export type InspectDroppedFile = (file: File) => Promise<DroppedPath | undefined>

export type FileActionsAPI = {
  canReveal: (backendUrl: string, sessionId: string) => Promise<boolean>
  reveal: (backendUrl: string, sessionId: string, path: string) => Promise<void>
}
