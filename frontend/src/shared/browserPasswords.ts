export const PASSWORD_CHANNEL = 'jaz:browser-passwords'
export const PASSWORD_CAPTURE_CHANNEL = 'jaz:browser-passwords:capture'
export const PASSWORD_USERNAME_CHANNEL = 'jaz:browser-passwords:username'
export const PASSWORD_FILL_CHANNEL = 'jaz:browser-passwords:fill'
export const PASSWORD_RESULT_CHANNEL = 'jaz:browser-passwords:result'
export const PASSWORD_CHANGED_CHANNEL = 'jaz:browser-passwords:changed'

export function passwordOrigin(value: string): string {
  try {
    const url = new URL(value)
    if (url.protocol === 'https:') {
      return url.origin
    }
    if (url.protocol === 'http:' && (url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]')) {
      return url.origin
    }
    return ''
  } catch {
    return ''
  }
}

export type BrowserPasswordEntry = {
  origin: string
  username: string
}

export type BrowserPasswordState = {
  origin: string
  usernames: string[]
  entries?: BrowserPasswordEntry[]
  locked: boolean
  pending?: { id: string; origin: string; username: string; update: boolean }
  error?: string
}

export type BrowserPasswordAction =
  | { kind: 'save' | 'dismiss'; id: string }
  | { kind: 'fill' | 'remove'; origin: string; username: string }
  | { kind: 'capture' | 'unlock' }

export type BrowserPasswordAPI = {
  state: (webContentsId: number) => Promise<BrowserPasswordState>
  act: (webContentsId: number, action: BrowserPasswordAction) => Promise<BrowserPasswordState>
  subscribe: (webContentsId: number, handler: () => void) => () => void
}
