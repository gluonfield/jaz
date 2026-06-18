import { BrowserWindow, type Input, type WebContents, webContents } from 'electron'
import {
  BROWSER_NAVIGATION_CHANNEL,
  type BrowserNavigationDirection,
} from '../shared/browserNavigation'

function appCommandDirection(command: string): BrowserNavigationDirection | null {
  if (command === 'browser-backward') return 'back'
  if (command === 'browser-forward') return 'forward'
  return null
}

function inputDirection(input: Input): BrowserNavigationDirection | null {
  if (input.type !== 'keyDown') return null
  if (input.key === 'BrowserBack') return 'back'
  if (input.key === 'BrowserForward') return 'forward'
  if (!input.meta || input.shift || input.control || input.alt) return null
  if (input.key === '[' || input.code === 'BracketLeft') return 'back'
  if (input.key === ']' || input.code === 'BracketRight') return 'forward'
  return null
}

function sendToRenderer(contents: WebContents, direction: BrowserNavigationDirection): boolean {
  const win = BrowserWindow.fromWebContents(contents)
  if (!win || win.isDestroyed()) return false
  win.webContents.send(BROWSER_NAVIGATION_CHANNEL, direction)
  return true
}

function navigateWebview(contents: WebContents, direction: BrowserNavigationDirection): boolean {
  if (contents.isDestroyed() || contents.getType() !== 'webview') return false
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

function navigateOrSend(contents: WebContents, direction: BrowserNavigationDirection): boolean {
  if (contents.getType() === 'webview') return navigateWebview(contents, direction)
  return sendToRenderer(contents, direction)
}

export function attachBrowserNavigationShortcuts(contents: WebContents): void {
  if (contents.getType() !== 'webview') return
  contents.on('before-input-event', (event, input) => {
    const direction = inputDirection(input)
    if (direction && navigateWebview(contents, direction)) event.preventDefault()
  })
}

export function attachBrowserNavigationCommands(win: BrowserWindow): void {
  win.on('app-command', (event, command) => {
    const direction = appCommandDirection(command)
    if (direction && navigateOrSend(focusedContentsForWindow(win), direction)) {
      event.preventDefault()
    }
  })
}
