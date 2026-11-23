export const FILE_ACTIONS_CHANNEL = 'jaz:file-actions'
export const DROPPED_DIRECTORY_CHANNEL = 'jaz:files:dropped-directory'

export type FileActionsAPI = {
  canReveal: (backendUrl: string, sessionId: string) => Promise<boolean>
  reveal: (backendUrl: string, sessionId: string, path: string) => Promise<void>
}
