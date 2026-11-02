import { expect, mock, spyOn, test } from 'bun:test'
import { spawnSync } from 'node:child_process'
import { EventEmitter } from 'node:events'
import { fileURLToPath } from 'node:url'

if (process.env.JAZ_UPDATER_TEST_CHILD === '1') {
  const app = { isPackaged: true }
  const powerMonitor = new EventEmitter()
  const autoUpdater = new EventEmitter()
  const nativeAutoUpdater = new EventEmitter()
  const handlers = new Map()
  const sent = []
  const timers = []
  let now = 0
  let checks = 0
  let terminate = async () => {}
  autoUpdater.checkForUpdates = () => {
    checks += 1
    return Promise.resolve(null)
  }
  mock.module('electron', () => ({ app, powerMonitor, autoUpdater: nativeAutoUpdater, ipcMain: { handle: (name, handler) => handlers.set(name, handler) } }))
  mock.module('electron-updater', () => ({ autoUpdater }))
  mock.module('./backend', () => ({ terminateLocalBackend: (...args) => terminate(...args) }))
  const { createUpdateController } = await import('./updater')

  function schedule(callback, delay, repeat) {
    timers.push({ callback, at: now + delay, repeat })
    return { unref() {} }
  }
  globalThis.setTimeout = (callback, delay) => schedule(callback, delay, 0)
  globalThis.setInterval = (callback, delay) => schedule(callback, delay, delay)

  async function advance(ms) {
    const target = now + ms
    for (;;) {
      const next = timers.filter((timer) => timer.at <= target).sort((a, b) => a.at - b.at)[0]
      if (!next) break
      now = next.at
      next.at = next.repeat ? next.at + next.repeat : Infinity
      next.callback()
      await Promise.resolve()
    }
    now = target
  }

  test('discovers updates every five minutes and after wake, retries failures and keeps ready updates', async () => {
    const controller = createUpdateController(() => ({ webContents: { send: (...args) => sent.push(args) } }))
    controller.start()
    controller.start()
    await advance(9_999)
    expect(checks).toBe(0)
    await advance(1)
    expect(checks).toBe(1)
    autoUpdater.emit('update-not-available')
    await advance(289_999)
    expect(checks).toBe(1)
    await advance(1)
    expect(checks).toBe(2)

    const log = spyOn(console, 'error').mockImplementation(() => {})
    autoUpdater.checkForUpdates = () => {
      checks += 1
      return Promise.reject(new Error('offline'))
    }
    try {
      await advance(300_000)
      expect(checks).toBe(3)
      expect(log).toHaveBeenCalledTimes(1)
    } finally {
      log.mockRestore()
    }
    autoUpdater.checkForUpdates = () => {
      checks += 1
      return Promise.resolve(null)
    }
    await advance(300_000)
    expect(checks).toBe(4)
    powerMonitor.emit('resume')
    expect(checks).toBe(5)
    autoUpdater.emit('update-available', { version: '0.0.159' })
    autoUpdater.emit('download-progress', { percent: 50 })
    await advance(300_000)
    powerMonitor.emit('resume')
    expect(checks).toBe(5)
    autoUpdater.emit('update-downloaded', { version: '0.0.159' })
    await advance(300_000)
    powerMonitor.emit('resume')
    controller.checkForUpdates()
    expect(checks).toBe(5)
    expect(sent.at(-1)).toEqual(['jaz:update-status', { state: 'downloaded', version: '0.0.159' }])
  })

  test('development apps do not start background update checks', () => {
    app.isPackaged = false
    const timerCount = timers.length
    const resumeCount = powerMonitor.listenerCount('resume')
    createUpdateController(() => null).start()
    expect(timers).toHaveLength(timerCount)
    expect(powerMonitor.listenerCount('resume')).toBe(resumeCount)
  })

  test('install IPC accepts once, publishes progress, preserves retry and ignores stale work', async () => {
    app.isPackaged = true
    autoUpdater.removeAllListeners()
    nativeAutoUpdater.removeAllListeners()
    const controller = createUpdateController(() => ({ webContents: { send: (...args) => sent.push(args) } }))
    controller.start()
    controller.registerIpc()
    const install = handlers.get('jaz:install-update')
    const getStatus = handlers.get('jaz:get-update-status')
    expect(install()).toEqual({ ok: false, error: 'update is not ready' })
    let rejectCheck
    autoUpdater.checkForUpdates = () => new Promise((_resolve, reject) => { rejectCheck = reject })
    controller.checkForUpdates()
    autoUpdater.emit('update-downloaded', { version: '0.0.159' })
    const pending = []
    terminate = (options, reason) => {
      expect(options).toEqual({ timeoutMs: 5_000, forceTimeoutMs: 1_000 })
      expect(reason).toBe('install_update')
      return new Promise((resolve, reject) => pending.push({ resolve, reject }))
    }
    let installs = 0
    autoUpdater.quitAndInstall = () => { installs += 1 }
    expect(install()).toEqual({ ok: true })
    expect(getStatus()).toEqual({ state: 'installing', version: '0.0.159' })
    expect(install()).toEqual({ ok: true })
    expect(pending).toHaveLength(1)
    expect(installs).toBe(0)
    const checksBefore = checks
    controller.checkForUpdates()
    powerMonitor.emit('resume')
    autoUpdater.emit('update-available', { version: 'stale' })
    autoUpdater.emit('download-progress', { percent: 1 })
    autoUpdater.emit('update-downloaded', { version: 'stale' })
    autoUpdater.emit('update-not-available')
    const reloaded = []
    controller.sendStatusTo({ webContents: { send: (...args) => reloaded.push(args) } })
    expect(reloaded).toEqual([['jaz:update-status', { state: 'installing', version: '0.0.159' }]])
    expect(checks).toBe(checksBefore)
    pending[0].resolve()
    await Promise.resolve()
    expect(installs).toBe(1)
    expect(install()).toEqual({ ok: true })
    expect(pending).toHaveLength(1)
    const log = spyOn(console, 'error').mockImplementation(() => {})
    try {
      rejectCheck(new Error('Stale check failed'))
      await Promise.resolve()
      expect(getStatus()).toEqual({ state: 'installing', version: '0.0.159' })
      autoUpdater.emit('error', new Error('Installer failed asynchronously'))
      expect(getStatus()).toEqual({ state: 'downloaded', version: '0.0.159', error: 'Installer failed asynchronously' })
      expect(nativeAutoUpdater.listenerCount('before-quit-for-update')).toBe(0)
      expect(install()).toEqual({ ok: true })
      pending[1].reject(new Error('Preparation failed'))
      await Promise.resolve()
      expect(getStatus()).toEqual({ state: 'downloaded', version: '0.0.159', error: 'Preparation failed' })
      expect(install()).toEqual({ ok: true })
      autoUpdater.emit('error', new Error('Preparation invalidated'))
      expect(install()).toEqual({ ok: true })
      pending[2].resolve()
      await Promise.resolve()
      expect(installs).toBe(1)
      autoUpdater.quitAndInstall = () => { throw new Error('Installer threw') }
      pending[3].resolve()
      await Promise.resolve()
      expect(getStatus()).toEqual({ state: 'downloaded', version: '0.0.159', error: 'Installer threw' })
      autoUpdater.quitAndInstall = () => { installs += 1 }
      expect(install()).toEqual({ ok: true })
      pending[4].resolve()
      await Promise.resolve()
      expect(installs).toBe(2)
      expect(getStatus()).toEqual({ state: 'installing', version: '0.0.159' })
    } finally {
      log.mockRestore()
    }
  })
} else {
  test('desktop updater scheduling in an isolated Electron fixture', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_UPDATER_TEST_CHILD: '1' },
      encoding: 'utf8',
      timeout: 10_000,
    })
    expect({ status: result.status, error: result.error?.message, output: result.status ? result.stderr : '' })
      .toEqual({ status: 0, error: undefined, output: '' })
  })
}
