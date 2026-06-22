import { appendFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { app, type BrowserWindow } from 'electron'

const WINDOW_SHOW_FALLBACK_MS = 3_000

type WindowLifecycleOptions = {
  label: string
  onDidFinishLoad?: () => void
}

function mainLogFile(): string | null {
  try {
    const dir = join(app.getPath('userData'), 'logs')
    mkdirSync(dir, { recursive: true })
    return join(dir, 'main.log')
  } catch {
    return null
  }
}

function errorText(error: unknown): string {
  if (error instanceof Error) return error.stack || error.message
  return String(error)
}

export function logMain(message: string, error?: unknown): void {
  const suffix = error === undefined ? '' : `\n${errorText(error)}`
  const line = `[${new Date().toISOString()}] ${message}${suffix}\n`
  process.stderr.write(line)
  const file = mainLogFile()
  if (!file) return
  try {
    appendFileSync(file, line)
  } catch {
    process.stderr.write(`[${new Date().toISOString()}] failed to write main log\n`)
  }
}

export function installMainDiagnostics(): void {
  process.on('uncaughtException', (error) => {
    logMain('uncaught exception', error)
    app.exit(1)
  })
  process.on('unhandledRejection', (error) => logMain('unhandled rejection', error))
}

export function attachWindowLifecycle(win: BrowserWindow, options: WindowLifecycleOptions): void {
  const { label, onDidFinishLoad } = options
  let shown = false

  const show = (reason: string): void => {
    if (win.isDestroyed()) return
    if (!win.isVisible()) win.show()
    if (shown || !win.isVisible()) return
    shown = true
    logMain(`${label} window shown (${reason})`)
  }

  win.webContents.on('did-fail-load', (_event, code, description, url, isMainFrame) => {
    if (isMainFrame) logMain(`${label} failed to load ${url}: ${code} ${description}`)
  })
  win.webContents.on('preload-error', (_event, preloadPath, error) => {
    logMain(`${label} preload failed at ${preloadPath}`, error)
  })
  win.webContents.on('render-process-gone', (_event, details) => {
    logMain(`${label} renderer exited: ${details.reason} ${details.exitCode}`)
  })
  win.webContents.on('console-message', (_event, level, message, line, sourceId) => {
    if (level >= 2) logMain(`${label} renderer console ${level}: ${message} (${sourceId}:${line})`)
  })
  win.on('unresponsive', () => logMain(`${label} window became unresponsive`))
  win.once('ready-to-show', () => show('ready-to-show'))
  win.webContents.once('did-finish-load', () => {
    logMain(`${label} window loaded`)
    show('did-finish-load')
    onDidFinishLoad?.()
  })

  const timer = setTimeout(() => {
    if (shown || win.isDestroyed()) return
    logMain(`${label} did not emit ready-to-show; showing fallback window`)
    show('fallback')
  }, WINDOW_SHOW_FALLBACK_MS)
  timer.unref()
  win.once('closed', () => clearTimeout(timer))
}
