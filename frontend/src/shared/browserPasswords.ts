export const PASSWORD_CHANNEL = 'jaz:browser-passwords'
export const PASSWORD_CAPTURE_CHANNEL = 'jaz:browser-passwords:capture'
export const PASSWORD_FILL_CHANNEL = 'jaz:browser-passwords:fill'
export const PASSWORD_RESULT_CHANNEL = 'jaz:browser-passwords:result'
export const PASSWORD_CHANGED_CHANNEL = 'jaz:browser-passwords:changed'

export type BrowserPasswordState = {
  origin: string
  usernames: string[]
  pending?: { id: string; username: string; update: boolean }
  error?: string
}

export type BrowserPasswordAction =
  | { kind: 'save' | 'dismiss'; id: string }
  | { kind: 'fill' | 'remove'; origin: string; username: string }
  | { kind: 'capture' }

export type BrowserPasswordAPI = {
  state: (webContentsId: number) => Promise<BrowserPasswordState>
  act: (webContentsId: number, action: BrowserPasswordAction) => Promise<BrowserPasswordState>
  subscribe: (webContentsId: number, handler: () => void) => () => void
}
