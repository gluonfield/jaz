export const FILE_ACTIONS_CHANNEL = 'jaz:file-actions'

export type FileActionsAPI = {
  canReveal: (backendUrl: string, sessionId: string) => Promise<boolean>
  reveal: (backendUrl: string, sessionId: string, path: string) => Promise<void>
}
