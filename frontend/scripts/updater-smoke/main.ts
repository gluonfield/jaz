import { app, autoUpdater as nativeAutoUpdater, BrowserWindow, ipcMain } from 'electron'
import { autoUpdater } from 'electron-updater'
import assert from 'node:assert/strict'
import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { createUpdateController } from '@main/updater'
import { startLocalBackend } from '@main/backend'
import { pending, preparations, useActualBackend } from './backend'
import * as fixtureProcess from './process'

const output = process.env.JAZ_UPDATER_SMOKE_DIR!
Object.defineProperty(app, 'isPackaged', { value: true })
type ClickDispatch = { point: { x: number; y: number }; target: string; hit: string | null }
let lastClick: ClickDispatch | undefined
let installs = 0
let heartbeats = 0
let paints = 0
let visibleWindows = 0
let focusedWindows = 0
let observations = 0
let lastHeartbeat = performance.now()
let longestHeartbeatGapMs = 0
const monitor = setInterval(() => {
  const now = performance.now()
  longestHeartbeatGapMs = Math.max(longestHeartbeatGapMs, now - lastHeartbeat)
  lastHeartbeat = now
  heartbeats += 1
  observations += 1
  for (const window of BrowserWindow.getAllWindows()) {
    visibleWindows += Number(window.isVisible())
    focusedWindows += Number(window.isFocused())
  }
}, 5)
autoUpdater.checkForUpdates = async () => null
autoUpdater.quitAndInstall = () => { installs += 1 }
const timeout = setTimeout(() => {
  console.error('Restart UI check timed out')
  fixtureProcess.child?.kill('SIGKILL')
  app.exit(1)
}, 30_000)

async function run() {
  await app.whenReady()
  const window = new BrowserWindow({
    width: 360,
    height: 260,
    show: false,
    focusable: false,
    webPreferences: { offscreen: true, backgroundThrottling: false, contextIsolation: true, sandbox: true, preload: join(output, 'index.js') },
  })
  const errors: string[] = []
  window.webContents.on('console-message', ({ level, message }) => {
    if (level === 'error') {
      errors.push(message)
      console.error('Renderer error:', message)
    }
  })
  window.webContents.on('paint', () => { paints += 1 })
  const controller = createUpdateController(() => window)
  controller.registerIpc()
  controller.start()
  assert.equal(autoUpdater.autoRunAppAfterInstall, true)
  const evaluate = <T>(code: string): Promise<T> => window.webContents.executeJavaScript(code)
  const delay = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))
  const composite = () => evaluate('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))')
  async function waitFor(code: string): Promise<void> {
    const deadline = performance.now() + 10_000
    while (!await evaluate<boolean>(code)) {
      assert.ok(performance.now() < deadline, `Timed out: ${code}`)
      await delay(5)
    }
  }
  async function click(text: string): Promise<void> {
    lastClick = await evaluate<ClickDispatch>(`(() => {
      const button = [...document.querySelectorAll('button')].find(button => button.textContent === ${JSON.stringify(text)})
      if (!button) throw new Error('Button missing')
      const describe = element => element instanceof Element
        ? element.tagName.slice(0, 32) + '#' + element.id.slice(0, 64) + ': ' + element.textContent.slice(0, 120)
        : null
      if (!window.updaterSmokeInput) {
        const received = window.updaterSmokeInput = []
        for (const type of ['pointerdown', 'pointerup', 'click']) {
          document.addEventListener(type, event => {
            received.push({ type, x: event.clientX, y: event.clientY, target: describe(event.target), trusted: event.isTrusted, buttons: event.buttons })
            if (received.length > 3) received.shift()
          }, { capture: true, passive: true })
        }
      }
      window.updaterSmokeInput.length = 0
      const bounds = button.getBoundingClientRect()
      const point = {x: Math.round(bounds.x + bounds.width / 2), y: Math.round(bounds.y + bounds.height / 2)}
      return {point, target: describe(button), hit: describe(document.elementFromPoint(point.x, point.y))}
    })()`)
    const { point } = lastClick
    window.webContents.sendInputEvent({ type: 'mouseDown', ...point, button: 'left', clickCount: 1 })
    window.webContents.sendInputEvent({ type: 'mouseUp', ...point, button: 'left', clickCount: 1 })
  }
  const installingDOM = `document.querySelector('[aria-busy="true"]') && [...document.querySelectorAll('button')].some(button => button.disabled && button.textContent === 'Restarting…')`
  const retryDOM = `document.body.textContent.includes('Restart failed') && [...document.querySelectorAll('button')].some(button => !button.disabled && button.textContent === 'Try again')`
  autoUpdater.emit('update-downloaded', { version: '0.0.999' })
  await window.loadFile(join(output, 'index.html'))
  await waitFor(`document.body.textContent.includes('Restart Jaz')`)
  await waitFor(`getComputedStyle(document.querySelector('[role="status"]')).opacity === '1'`)
  await composite()
  const firstClick = performance.now()
  await click('Restart Jaz')
  await waitFor(installingDOM)
  const progressMs = performance.now() - firstClick
  assert.equal(await evaluate(`getComputedStyle(document.querySelector('[role="status"]')).opacity`), '1')
  await composite()
  await writeFile(join(output, 'first-progress.png'), (await window.webContents.capturePage()).toPNG())
  const progressPaintMs = performance.now() - firstClick
  await evaluate(`Promise.all(Array.from({length: 8}, () => window.jaz.installUpdate()))`)
  assert.equal(preparations, 1)
  assert.equal(installs, 0)
  assert.deepEqual(await evaluate('window.jaz.getUpdateStatus()'), { state: 'installing', version: '0.0.999' })
  autoUpdater.emit('download-progress', { percent: 1 })
  autoUpdater.emit('update-not-available')
  await waitFor(installingDOM)
  const reloaded = new Promise((resolve) => window.webContents.once('did-finish-load', resolve))
  window.webContents.reload()
  await reloaded
  await waitFor(installingDOM)
  await waitFor(`getComputedStyle(document.querySelector('[role="status"]')).opacity === '1'`)
  await composite()
  await writeFile(join(output, 'preparing.png'), (await window.webContents.capturePage()).toPNG())
  pending[0].reject(new Error('Preparation refused'))
  await waitFor(retryDOM)
  assert.ok(await evaluate(`document.body.textContent.includes('Preparation refused')`))
  await click('Try again')
  await waitFor(installingDOM)
  pending[1].resolve()
  await delay(20)
  assert.equal(installs, 1)
  autoUpdater.emit('error', new Error('Installer failed after handoff'))
  await waitFor(retryDOM)
  assert.equal(nativeAutoUpdater.listenerCount('before-quit-for-update'), 0)
  assert.ok(await evaluate(`document.body.textContent.includes('Installer failed after handoff')`))
  await composite()
  await writeFile(join(output, 'retry.png'), (await window.webContents.capturePage()).toPNG())

  ipcMain.removeHandler('jaz:install-update')
  await click('Try again')
  await waitFor(`${retryDOM} && document.body.textContent.includes('No handler registered')`)
  ipcMain.removeHandler('jaz:get-update-status')
  controller.registerIpc()
  await click('Try again')
  await waitFor(installingDOM)
  assert.equal(preparations, 3)
  pending[2].resolve()
  await delay(20)
  assert.equal(installs, 2)
  autoUpdater.emit('error', new Error('Retry fixture reset'))
  await waitFor(retryDOM)

  let healthCalls = 0
  globalThis.fetch = async () => new Response(JSON.stringify({ ok: ++healthCalls > 1, auth_required: false }))
  useActualBackend()
  assert.equal((await startLocalBackend()).ok, true)
  await fixtureProcess.ready
  const shutdownStart = performance.now()
  longestHeartbeatGapMs = 0
  lastHeartbeat = shutdownStart
  const paintsBefore = paints
  const heartbeatsBefore = heartbeats
  await click('Try again')
  await waitFor(installingDOM)
  const shutdownProgressMs = performance.now() - shutdownStart
  const ipcStart = performance.now()
  assert.deepEqual(await evaluate('window.jaz.getUpdateStatus()'), { state: 'installing', version: '0.0.999' })
  const ipcDuringShutdownMs = performance.now() - ipcStart
  await click('Interactions: 0')
  await waitFor(`document.querySelector('#interaction').textContent === 'Interactions: 1'`)
  await evaluate(`document.querySelector('input').focus()`)
  window.webContents.insertText('Draft still works')
  await waitFor(`document.querySelector('input').value === 'Draft still works'`)
  await delay(100)
  assert.equal(installs, 2)
  await writeFile(join(output, 'responsive.png'), (await window.webContents.capturePage()).toPNG())
  while (installs === 2) {
    assert.ok(performance.now() - shutdownStart < 8_000)
    await delay(10)
  }
  const preparationMs = performance.now() - shutdownStart
  const paintsDuringShutdown = paints - paintsBefore
  const heartbeatsDuringShutdown = heartbeats - heartbeatsBefore
  assert.equal(installs, 3)
  assert.equal(preparations, 4)
  assert.equal(fixtureProcess.child.signalCode, 'SIGKILL')
  assert.equal(visibleWindows, 0)
  assert.equal(focusedWindows, 0)
  assert.deepEqual(errors, [])
  await writeFile(join(output, 'report.json'), JSON.stringify({
    progressMs,
    progressPaintMs,
    shutdownProgressMs,
    ipcDuringShutdownMs,
    preparationMs,
    paintsDuringShutdown,
    heartbeatsDuringShutdown,
    longestHeartbeatGapMs,
    visibleWindows,
    focusedWindows,
    observations,
    installs,
    preparations,
    shutdown: 'Production terminateLocalBackend; synthetic owned child ignores SIGTERM and is killed after the unchanged 5-second grace period.',
    otherFailures: 'Deferred preparation rejection, asynchronous installer error, missing install IPC handler; updater calls and health responses are fixture boundaries.',
  }, null, 2))
  window.destroy()
}

void run().then(() => {
  clearInterval(monitor)
  clearTimeout(timeout)
  app.exit(0)
}).catch(async (error) => {
  console.error(error)
  const snapshot: { renderer?: unknown; updateStatus?: unknown } = {}
  const capture = (async () => {
    const window = BrowserWindow.getAllWindows()[0]
    if (!window) throw new Error('Fixture window unavailable')
    snapshot.renderer = await window.webContents.executeJavaScript(`({
      received: window.updaterSmokeInput ?? [],
      updateText: document.querySelector('[role="status"], [role="alert"]')?.textContent.slice(0, 240),
      interaction: document.querySelector('#interaction')?.textContent.slice(0, 120),
      focused: document.hasFocus(),
    })`)
    snapshot.updateStatus = await window.webContents.executeJavaScript(`window.jaz.getUpdateStatus().then(({state, version, error, message}) => ({
      state, version: version?.slice(0, 64), error: error?.slice(0, 240), message: message?.slice(0, 240),
    }))`)
  })()
  const captureError = await Promise.race([
    capture.then(() => null, (error) => String(error).slice(0, 240)),
    new Promise<string>((resolve) => setTimeout(() => resolve('Renderer/IPC snapshot timed out after 500ms'), 500)),
  ])
  console.error('Restart UI failure evidence:', JSON.stringify({
    dispatch: lastClick,
    ...snapshot,
    captureError,
    installs,
    preparations,
    heartbeats,
    paints,
    lastHeartbeatAgeMs: performance.now() - lastHeartbeat,
    longestHeartbeatGapMs,
    observations,
    visibleWindows,
    focusedWindows,
  }))
  fixtureProcess.child?.kill('SIGKILL')
  clearInterval(monitor)
  clearTimeout(timeout)
  app.exit(1)
})
