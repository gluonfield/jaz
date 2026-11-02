import { app, autoUpdater as nativeAutoUpdater, type BrowserWindow, ipcMain, powerMonitor } from 'electron'
import { autoUpdater } from 'electron-updater'
import type { UpdateStatus } from '../shared/update'
import { terminateLocalBackend } from './backend'

const UPDATE_CHECK_INTERVAL_MS = 5 * 60 * 1000

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

  function currentVersion(): string | undefined {
    return 'version' in status ? status.version : undefined
  }

  function setStatus(next: UpdateStatus): void {
    status = next
    getMainWindow()?.webContents.send('jaz:update-status', next)
  }

  function downloadStatus(next: UpdateStatus): void {
    if (status.state !== 'installing') setStatus(next)
  }

  function check(manual = false): void {
    if (!app.isPackaged) {
      if (manual) setStatus({ state: 'error', message: 'Update checks require a packaged app.' })
      return
    }
    if (status.state === 'checking' || status.state === 'downloading' || status.state === 'downloaded' || status.state === 'installing') {
      return
    }
    if (manual) setStatus({ state: 'checking' })
    void autoUpdater.checkForUpdates().catch((err: unknown) => {
      console.error('Jaz update check failed', err)
      if (manual || status.state === 'checking') {
        downloadStatus({ state: 'error', message: err instanceof Error ? err.message : String(err) })
      }
    })
  }

  function exitForUpdate(): void {
    app.exit(0)
  }

  function fail(message: string): void {
    nativeAutoUpdater.removeListener('before-quit-for-update', exitForUpdate)
    if (status.state === 'installing' || status.state === 'downloaded') {
      setStatus({ state: 'downloaded', version: status.version, error: message })
    } else {
      setStatus({ state: 'error', message })
    }
  }

  async function installDownloaded(installing: UpdateStatus): Promise<void> {
    try {
      await terminateLocalBackend({ timeoutMs: 5_000, forceTimeoutMs: 1_000 }, 'install_update')
      if (status !== installing) return
      if (process.platform === 'darwin') {
        nativeAutoUpdater.once('before-quit-for-update', exitForUpdate)
      }
      autoUpdater.quitAndInstall()
    } catch (err) {
      if (status === installing) fail(err instanceof Error ? err.message : String(err))
    }
  }

  function install(): InstallUpdateResult {
    if (status.state === 'installing') return { ok: true }
    if (status.state !== 'downloaded') return { ok: false, error: 'update is not ready' }
    const installing: UpdateStatus = { state: 'installing', version: status.version }
    setStatus(installing)
    void installDownloaded(installing)
    return { ok: true }
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
        downloadStatus({ state: 'available', version: updateVersion(info) }),
      )
      autoUpdater.on('download-progress', (info) => {
        const percent = Number.isFinite(info.percent) ? Math.max(0, Math.min(100, info.percent)) : 0
        downloadStatus({ state: 'downloading', percent, version: currentVersion() })
      })
      autoUpdater.on('update-downloaded', (info) =>
        downloadStatus({ state: 'downloaded', version: updateVersion(info) }),
      )
      autoUpdater.on('update-not-available', () => downloadStatus({ state: 'idle' }))
      autoUpdater.on('error', (err) => {
        console.error('Jaz updater error', err)
        if (status.state !== 'idle') {
          fail(err instanceof Error ? err.message : String(err))
        }
      })
      setTimeout(check, 10_000).unref()
      timer = setInterval(check, UPDATE_CHECK_INTERVAL_MS)
      timer.unref()
      powerMonitor.on('resume', () => check())
    },
  }
}
