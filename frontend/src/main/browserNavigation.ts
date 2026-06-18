import { BrowserWindow, type Input, type WebContents, webContents } from 'electron'

type NavigationDirection = 'back' | 'forward'

function appCommandDirection(command: string): NavigationDirection | null {
  if (command === 'browser-backward') return 'back'
  if (command === 'browser-forward') return 'forward'
  return null
}

function inputDirection(input: Input): NavigationDirection | null {
  if (input.type !== 'keyDown') return null
  if (input.key === 'BrowserBack') return 'back'
  if (input.key === 'BrowserForward') return 'forward'
  if (!input.meta || input.shift || input.control || input.alt) return null
  if (input.key === '[' || input.code === 'BracketLeft') return 'back'
  if (input.key === ']' || input.code === 'BracketRight') return 'forward'
  return null
}

function navigable(contents: WebContents): boolean {
  return contents.getType() === 'webview' || BrowserWindow.fromWebContents(contents) !== null
}

function navigate(contents: WebContents, direction: NavigationDirection): boolean {
  if (contents.isDestroyed() || !navigable(contents)) return false
  const history = contents.navigationHistory
  if (direction === 'back') {
    if (!history.canGoBack()) return false
    history.goBack()
    return true
  }
  if (!history.canGoForward()) return false
  history.goForward()
  return true
}

function focusedContentsForWindow(win: BrowserWindow): WebContents {
  const focused = webContents.getFocusedWebContents()
  if (!focused || focused.isDestroyed()) return win.webContents
  const host = focused.hostWebContents ?? focused
  return BrowserWindow.fromWebContents(host) === win ? focused : win.webContents
}

export function attachBrowserNavigationShortcuts(contents: WebContents): void {
  contents.on('before-input-event', (event, input) => {
    const direction = inputDirection(input)
    if (direction && navigate(contents, direction)) event.preventDefault()
  })
}

export function attachBrowserNavigationCommands(win: BrowserWindow): void {
  win.on('app-command', (event, command) => {
    const direction = appCommandDirection(command)
    if (direction && navigate(focusedContentsForWindow(win), direction)) event.preventDefault()
  })
}
