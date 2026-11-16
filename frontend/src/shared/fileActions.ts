export const FILE_ACTIONS_CHANNEL = 'jaz:file-actions'

export type FileActionsAPI = {
  canReveal: (sessionId: string) => Promise<boolean>
  reveal: (sessionId: string, path: string) => Promise<void>
}
