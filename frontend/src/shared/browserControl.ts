export const BROWSER_COMMAND_CHANNEL = 'jaz:browser-command'

export type BrowserCommand = {
  method: string
  params?: Record<string, unknown>
}

export type BrowserCommandRequest = BrowserCommand & {
  webContentsId: number
}

export const BROWSER_CDP_METHODS = new Set([
  'Page.enable',
  'Page.navigate',
  'Page.getFrameTree',
  'Page.createIsolatedWorld',
  'Page.captureScreenshot',
  'Runtime.enable',
  'Runtime.evaluate',
  'Input.dispatchMouseEvent',
  'Input.dispatchKeyEvent',
  'Input.insertText',
])

export const BROWSER_COMMAND_METHODS = new Set([
  'Jaz.cursorScale',
  'Jaz.navigate',
  ...BROWSER_CDP_METHODS,
])
