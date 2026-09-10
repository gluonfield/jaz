export const BROWSER_COMMAND_CHANNEL = 'jaz:browser-command'

export type BrowserCommand = {
  method: string
  params?: Record<string, unknown>
  sessionId?: string
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
  'Accessibility.enable',
  'Accessibility.getFullAXTree',
  'Accessibility.getPartialAXTree',
  'Input.dispatchMouseEvent',
  'Input.dispatchKeyEvent',
  'Input.insertText',
])

export const BROWSER_COMMAND_METHODS = new Set([
  'Jaz.cursorScale',
  'Jaz.navigate',
  'Jaz.frames',
  'DOM.resolveNode',
  'DOM.getFrameOwner',
  'DOM.getBoxModel',
  'DOM.getNodeForLocation',
  'Runtime.callFunctionOn',
  'Runtime.releaseObject',
  ...BROWSER_CDP_METHODS,
])
