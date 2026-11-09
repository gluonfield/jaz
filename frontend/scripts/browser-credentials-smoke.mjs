import { app, BrowserWindow } from 'electron'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import process from 'node:process'
import console from 'node:console'
import { clearTimeout, setTimeout } from 'node:timers'

const output = process.env.JAZ_BROWSER_CREDENTIAL_TEST_DIR
if (process.platform === 'darwin') app.setActivationPolicy('prohibited')
app.disableHardwareAcceleration()
app.setPath('userData', join(output, 'profile'))
let stage = 'startup'
const deadline = setTimeout(() => {
  console.error('Browser credential check timed out:', stage)
  app.exit(1)
}, 20_000)

async function run() {
  const script = await readFile(join(output, 'renderer.js'))
  stage = 'listen'
  let approvedChecks = 0
  const server = createServer((request, response) => {
    response.setHeader('Content-Type', 'application/json')
    if (request.url === '/health') return response.end(JSON.stringify({ ok: true, auth_required: true }))
    if (request.url === '/v1/auth/check') {
      if (request.headers.authorization !== 'Bearer jaz_fixture') {
        response.statusCode = 401
        return response.end(JSON.stringify({ code: 'unauthorized' }))
      }
      approvedChecks += 1
      return response.end(JSON.stringify({ ok: true, auth_kind: 'device', device_id: 'device' }))
    }
    if (request.url === '/fixture.js') {
      response.setHeader('Content-Type', 'text/javascript')
      return response.end(script)
    }
    response.setHeader('Content-Type', 'text/html')
    response.end('<!doctype html><script src="/fixture.js"></script>')
  })
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const origin = `http://127.0.0.1:${server.address().port}`
  stage = 'app ready'
  await app.whenReady()
  const window = new BrowserWindow({ show: false, focusable: false, webPreferences: { offscreen: true, sandbox: true, backgroundThrottling: false } })
  let visibleWindows = 0
  let focusedWindows = 0
  window.on('show', () => { visibleWindows += 1 })
  window.on('focus', () => { focusedWindows += 1 })
  const evaluate = (code) => window.webContents.executeJavaScript(code)
  stage = 'load page'
  await window.loadURL(origin)
  stage = 'parallel key creation'
  assert.deepEqual(await evaluate(`(async () => {
    await Promise.all([
      credentials.saveBrowserCredential('https://one.example', 'first', 'one', () => true),
      credentials.saveBrowserCredential('https://two.example', 'second', 'two', () => true),
    ])
    const db = await new Promise(resolve => {
      const request = indexedDB.open('jaz.browserCredentials', 1)
      request.onsuccess = () => resolve(request.result)
    })
    const tx = db.transaction('credentials', 'readwrite')
    const store = tx.objectStore('credentials')
    const key = store.get('key')
    const record = store.get('https://one.example')
    record.onsuccess = () => store.put(record.result, 'https://copy.example')
    await new Promise(resolve => { tx.oncomplete = resolve })
    db.close()
    let originBound = false
    try { await credentials.loadBrowserCredential('https://copy.example') } catch { originBound = true }
    return { nonExportable: !key.result.extractable, encrypted: record.result.ciphertext.byteLength === new TextEncoder().encode('first').byteLength + 16, originBound }
  })()`), { nonExportable: true, encrypted: true, originBound: true })
  await window.loadURL(origin + '/reload')
  stage = 'restore after reload'
  assert.deepEqual(await evaluate(`Promise.all(['https://one.example', 'https://two.example'].map(url => credentials.loadBrowserCredential(url)))`), [
    { token: 'first', deviceId: 'one' },
    { token: 'second', deviceId: 'two' },
  ])
  stage = 'hold storage transaction'
  await evaluate(`new Promise(resolve => {
    const request = indexedDB.open('jaz.browserCredentials', 1)
    request.onsuccess = () => {
      const db = request.result
      const tx = db.transaction('credentials', 'readwrite')
      const store = tx.objectStore('credentials')
      let held = true
      window.releaseStorage = () => { held = false; db.close() }
      const hold = () => { if (held) store.get('key').onsuccess = hold }
      hold()
      resolve()
    }
  })`)
  stage = 'connect with held storage'
  assert.equal(await evaluate(`Promise.race([
    connection.connectRemote(location.origin + '#key=jaz_fixture').then(() => Boolean(api.cachedApiAuthToken())),
    new Promise(resolve => setTimeout(() => resolve(false), 1000)),
  ])`), true)
  assert.equal(await evaluate('connection.connectionPreference()'), null)
  stage = 'forget with held storage'
  await evaluate(`window.cancelled = false
    window.cancelledSave = credentials.saveBrowserCredential('https://cancel.example', 'cancelled', 'device', () => !window.cancelled).then(() => false, () => true)
    connection.forgetBackend(location.origin)
    window.cancelled = true
    releaseStorage()`)
  assert.equal(await evaluate('cancelledSave'), true)
  assert.equal(await evaluate('credentials.loadBrowserCredential("https://cancel.example")'), undefined)
  assert.equal(await evaluate('credentials.loadBrowserCredential(location.origin)'), undefined)
  assert.equal(await evaluate('connection.connectionPreference()'), null)
  assert.equal(await evaluate('Boolean(api.cachedApiAuthToken())'), false)
  stage = 'save an approved connection'
  await evaluate(`(async () => {
    await connection.connectRemote(location.origin + '#key=jaz_fixture')
    while (connection.connectionPreference()?.remoteUrl !== location.origin) {
      await new Promise(resolve => setTimeout(resolve, 10))
    }
  })()`)
  const checksBeforeReload = approvedChecks
  stage = 'restore and revalidate the approved connection'
  await window.loadURL(origin + '/reopen')
  assert.equal(await evaluate(`(async () => {
    while (!api.cachedApiAuthToken(location.origin)) {
      await new Promise(resolve => setTimeout(resolve, 10))
    }
    return api.cachedApiAuthToken(location.origin)
  })()`), 'jaz_fixture')
  assert.ok(approvedChecks > checksBeforeReload)
  assert.equal(window.isVisible(), false)
  assert.equal(window.isFocused(), false)
  await writeFile(join(output, 'report.json'), JSON.stringify({ visibleWindows, focusedWindows }))
  window.destroy()
  server.close()
  clearTimeout(deadline)
  app.quit()
}

void run().catch((error) => {
  console.error(error)
  app.exit(1)
})
