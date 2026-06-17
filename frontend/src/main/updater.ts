import { app, autoUpdater as nativeAutoUpdater, type BrowserWindow, ipcMain } from 'electron'
import { autoUpdater } from 'electron-updater'
import type { UpdateStatus } from '../shared/update'
import { terminateLocalBackend } from './backend'

const UPDATE_CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000

type InstallUpdateResult = {
  ok: boolean
  error?: string
}

function updateVersion(info: { version?: string }): string | undefined {
  return typeof info.version === 'string' && info.version !== '' ? info.version : undefined
}

export function createUpdateController(getMainWindow: () => BrowserWindow | null) {
  let timer: NodeJS.Timeout | null = null
  let status: UpdateStatus = { state: 'idle' }
  let installStarted = false

  function currentVersion(): string | undefined {
    return 'version' in status ? status.version : undefined
  }

  function setStatus(next: UpdateStatus): void {
    status = next
    getMainWindow()?.webContents.send('jaz:update-status', next)
  }

  function check(manual = false): void {
    if (!app.isPackaged) {
      if (manual) setStatus({ state: 'error', message: 'Update checks require a packaged app.' })
      return
    }
    if (status.state === 'checking' || status.state === 'downloading' || status.state === 'downloaded') {
      return
    }
    if (manual) setStatus({ state: 'checking' })
    void autoUpdater.checkForUpdates().catch((err: unknown) => {
      console.error('Jaz update check failed', err)
      if (manual || status.state === 'checking') {
        setStatus({ state: 'error', message: err instanceof Error ? err.message : String(err) })
      }
    })
  }

  async function installDownloaded(): Promise<void> {
    await terminateLocalBackend({ timeoutMs: 5_000, forceTimeoutMs: 1_000 })
    if (process.platform === 'darwin') {
      nativeAutoUpdater.once('before-quit-for-update', () => app.exit(0))
    }
    autoUpdater.quitAndInstall()
  }

  async function install(): Promise<InstallUpdateResult> {
    if (status.state !== 'downloaded') return { ok: false, error: 'update is not ready' }
    if (installStarted) return { ok: true }
    installStarted = true
    try {
      await installDownloaded()
      return { ok: true }
    } catch (err) {
      installStarted = false
      const message = err instanceof Error ? err.message : String(err)
      setStatus({ state: 'error', message })
      return { ok: false, error: message }
    }
  }

  return {
    sendStatusTo(win: BrowserWindow): void {
      if (status.state !== 'idle') win.webContents.send('jaz:update-status', status)
    },

    checkForUpdates(): void {
      check(true)
    },

    registerIpc(): void {
      ipcMain.handle('jaz:get-update-status', () => status)
      ipcMain.handle('jaz:install-update', () => install())
    },

    start(): void {
      if (!app.isPackaged || timer) return
      autoUpdater.autoDownload = true
      autoUpdater.autoInstallOnAppQuit = false
      autoUpdater.on('update-available', (info) =>
        setStatus({ state: 'available', version: updateVersion(info) }),
      )
      autoUpdater.on('download-progress', (info) => {
        const percent = Number.isFinite(info.percent) ? Math.max(0, Math.min(100, info.percent)) : 0
        setStatus({ state: 'downloading', percent, version: currentVersion() })
      })
      autoUpdater.on('update-downloaded', (info) =>
        setStatus({ state: 'downloaded', version: updateVersion(info) }),
      )
      autoUpdater.on('update-not-available', () => setStatus({ state: 'idle' }))
      autoUpdater.on('error', (err) => {
        console.error('Jaz updater error', err)
        if (status.state !== 'idle') {
          setStatus({
            state: 'error',
            message: err instanceof Error ? err.message : String(err),
          })
        }
      })
      setTimeout(check, 10_000).unref()
      timer = setInterval(check, UPDATE_CHECK_INTERVAL_MS)
      timer.unref()
    },
  }
}
