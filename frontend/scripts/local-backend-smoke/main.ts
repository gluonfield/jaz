import { app, BrowserWindow, session } from 'electron'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { gzipSync } from 'node:zlib'
import { configureLocalBackendTransport, registerLocalBackendTransport } from '@main/localBackendTransport'
import { configureAppPermissions } from '@main/permissions'
import { LOCAL_BACKEND_TRANSPORT_URL, LOCAL_BACKEND_URL } from '@shared/localBackend'
import type { SessionMessages } from '@/lib/api/types'

const output = process.env.JAZ_LOCAL_BACKEND_TEST_DIR!
if (process.platform === 'darwin') app.setActivationPolicy('prohibited')
app.disableHardwareAcceleration()
app.setPath('userData', join(output, 'profile'))
registerLocalBackendTransport()
let stage = 'startup'

async function run() {
  let healthy = true
  let cancelled = false
  let hanging = false
  let forbiddenHits = 0
  let redirectedAuthorization = ''
  let remoteMutations = 0
  let visibleWindows = 0
  let focusedWindows = 0
  const server = createServer(async (request, response) => {
    const url = new URL(request.url!, 'http://fixture')
    const path = url.pathname
    response.setHeader('Cache-Control', 'no-store')
    response.setHeader('Content-Type', 'application/json')
    const origin = request.headers.origin
    if (origin && origin !== 'null') {
      response.writeHead(403)
      response.end('origin is not allowed')
      return
    }
    if (origin) response.setHeader('Access-Control-Allow-Origin', origin)
    response.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type, X-Jaz-Client-Platform')
    if (request.method === 'OPTIONS') {
      response.writeHead(204)
      response.end()
      return
    }
    if (path === '/health') {
      response.end(JSON.stringify({ ok: healthy, auth_required: true }))
      return
    }
    if (path === '/page') {
      response.setHeader('Content-Type', 'text/html')
      response.end('<!doctype html><title>Untrusted origin</title>')
      return
    }
    if (path === '/fixture/forbidden') forbiddenHits += 1
    if (path === '/fixture/remote-mutate') remoteMutations += 1
    if (path === '/fixture/redirected') {
      redirectedAuthorization = request.headers.authorization || ''
      response.end('{"redirected":true}')
      return
    }
    const resource = path === '/fixture/image' || path === '/fixture/events'
    if (resource ? url.searchParams.get('ticket') !== 'fixture-ticket' : request.headers.authorization !== 'Bearer jaz_fixture') {
      response.writeHead(401)
      response.end('{"code":"unauthorized"}')
      return
    }
    if (path === '/fixture/redirect') {
      response.writeHead(302, { Location: backend.replace('127.0.0.1', 'localhost') + '/fixture/redirected' })
      response.end()
      return
    }
    if (path === '/fixture/local-redirect') {
      response.writeHead(302, { Location: '/v1/sessions/stored-chat/messages' })
      response.end()
      return
    }
    if (path === '/fixture/redirect-method') {
      response.writeHead(Number(url.searchParams.get('status')), { Location: '/fixture/method' })
      response.end()
      return
    }
    if (path === '/fixture/image') {
      response.setHeader('Content-Type', 'image/svg+xml')
      response.end('<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"/>')
      return
    }
    if (path === '/fixture/events') {
      response.setHeader('Content-Type', 'text/event-stream')
      response.write('data: local-event\n\n')
      return
    }
    if (path === '/v1/auth/check') {
      response.end(JSON.stringify({ ok: true, auth_kind: 'device', device_id: 'fixture', resource_ticket: { token: 'fixture-ticket', expires_at: new Date(Date.now() + 60_000).toISOString() } }))
      return
    }
    if (path === '/v1/sessions/stored-chat/messages') {
      const at = '2026-11-13T00:00:00Z'
      const transcript: SessionMessages = {
        session: { id: 'stored-chat', slug: 'stored-chat', runtime: 'acp', status: 'idle', created_at: at, updated_at: at, last_attention_at: at },
        messages: [{ seq: 1, role: 'user', content: 'Stored chat on disk', blocks: [], created_at: at }],
        events: [], has_earlier: false, history_revision: 1, latest_event_seq: 0,
      }
      response.setHeader('Content-Encoding', 'gzip')
      response.end(gzipSync(JSON.stringify(transcript)))
      return
    }
    if (path === '/fixture/mutate') {
      const chunks = []
      for await (const chunk of request) chunks.push(chunk)
      response.end(Buffer.concat(chunks).toString())
      return
    }
    if (path === '/fixture/method') {
      const chunks = []
      for await (const chunk of request) chunks.push(chunk)
      response.end(JSON.stringify({ method: request.method, body: Buffer.concat(chunks).toString() }))
      return
    }
    if (path === '/fixture/hang') {
      hanging = true
      response.flushHeaders()
      response.on('close', () => { cancelled = true })
      return
    }
    response.writeHead(503)
    response.end('{"error":"Unavailable"}')
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  assert.ok(address && typeof address !== 'string')
  const backend = `http://127.0.0.1:${address.port}`
  await app.whenReady()
  configureAppPermissions(session.defaultSession)
  configureLocalBackendTransport(session.defaultSession, backend)
  const window = new BrowserWindow({ show: false, focusable: false, webPreferences: { preload: join(output, 'preload.cjs'), offscreen: true, backgroundThrottling: false } })
  window.webContents.on('console-message', ({ level, message }) => {
    if (level === 'error') console.error(message)
  })
  window.on('show', () => { visibleWindows += 1 })
  window.on('focus', () => { focusedWindows += 1 })
  const evaluate = (code: string) => window.webContents.executeJavaScript(code)
  const until = async (check: () => Promise<boolean> | boolean) => {
    const end = Date.now() + 18_000
    while (!await check()) {
      assert.ok(Date.now() < end, stage)
      await new Promise((resolve) => setTimeout(resolve, 25))
    }
  }
  stage = 'connect'
  await window.loadFile(join(output, 'index.html'))
  await until(() => evaluate('Boolean(window.fixture)'))
  await until(() => evaluate('document.documentElement.dataset.status === "disconnected"'))
  assert.equal(await evaluate('fixture.connect()'), null)
  await until(() => evaluate('document.documentElement.dataset.status === "connected"'))
  assert.equal(await evaluate('fixture.apiBaseUrl()'), LOCAL_BACKEND_URL)
  assert.equal(await evaluate(`fixture.apiUrl('/v1/hooks/shareable')`), LOCAL_BACKEND_URL + '/v1/hooks/shareable')
  stage = 'offline chat and mutation'
  window.webContents.debugger.attach()
  await window.webContents.debugger.sendCommand('Network.enable')
  await window.webContents.debugger.sendCommand('Network.emulateNetworkConditions', { offline: true, latency: 0, downloadThroughput: 0, uploadThroughput: 0 })
  await until(() => evaluate('!navigator.onLine && fixture.offline()'))
  assert.equal((await evaluate('fixture.health()')).ok, true)
  assert.equal((await evaluate(`Promise.race([
    fixture.chat(),
    new Promise((_, reject) => setTimeout(() => reject(new Error('Local chat was paused offline')), 3000)),
  ])`)).messages[0].content, 'Stored chat on disk')
  assert.equal((await evaluate(`fixture.get('/fixture/local-redirect')`)).messages[0].content, 'Stored chat on disk')
  assert.deepEqual(await evaluate('fixture.mutate()'), { seen: true })
  for (const status of [301, 302, 303, 307, 308]) {
    assert.deepEqual(await evaluate(`fixture.post('/fixture/redirect-method?status=${status}', {seen:true})`), status < 307 ? { method: 'GET', body: '' } : { method: 'POST', body: '{"seen":true}' })
  }
  assert.equal(await evaluate(`fixture.get('/fixture/unavailable').then(() => 0, error => error.status)`), 503)
  stage = 'offline streaming and embedded resources'
  assert.equal(await evaluate('fixture.stream()'), 'local-event')
  assert.equal(await evaluate(`new Promise(resolve => {
    const image = new Image()
    image.onload = () => resolve(image.naturalWidth)
    image.onerror = () => resolve(0)
    image.src = fixture.embedded()
  })`), 4)
  stage = 'cancellation'
  await evaluate(`void (window.hanging = fetch(fixture.backendTransportUrl(${JSON.stringify(LOCAL_BACKEND_URL + '/fixture/hang')}), {headers:{Authorization:'Bearer jaz_fixture'}, signal:(window.abort = new AbortController()).signal}).then(response => response.text()).catch(error => error.name))`)
  await until(() => hanging)
  await evaluate('abort.abort()')
  assert.equal(await evaluate('hanging'), 'AbortError')
  await until(() => cancelled)
  stage = 'remote transport and redirects remain offline'
  assert.equal(await evaluate(`fetch(${JSON.stringify(backend + '/health')}).then(() => true, () => false)`), false)
  assert.equal(await evaluate(`fixture.remoteMutation(${JSON.stringify(backend)})`), false)
  assert.equal(await evaluate(`fixture.get('/fixture/redirect').then(() => true, () => false)`), false)
  assert.equal(redirectedAuthorization, '')
  assert.equal(remoteMutations, 0)
  stage = 'real outage and recovery'
  healthy = false
  await until(() => evaluate('document.documentElement.dataset.status === "reconnecting" && document.body.textContent.includes("Reconnecting to backend")'))
  healthy = true
  await until(() => evaluate('document.documentElement.dataset.status === "connected"'))
  stage = 'reject other origins, opaque frames and other targets'
  const untrusted = new BrowserWindow({ show: false, focusable: false, webPreferences: { offscreen: true } })
  await untrusted.loadURL(backend + '/page')
  assert.notEqual(await untrusted.webContents.executeJavaScript(`fetch(${JSON.stringify(LOCAL_BACKEND_TRANSPORT_URL + '/fixture/forbidden')}).then(response => response.status, () => 0)`), 200)
  assert.equal(await evaluate(`fetch('jaz-local://other/fixture/forbidden').then(response => response.status)`), 403)
  await evaluate(`new Promise(resolve => {
    addEventListener('message', event => { if (event.data === 'opaque-done') resolve() }, {once:true})
    const frame = document.createElement('iframe')
    frame.sandbox = 'allow-scripts'
    frame.srcdoc = ${JSON.stringify(`<script>fetch('${LOCAL_BACKEND_TRANSPORT_URL}/fixture/forbidden').finally(() => parent.postMessage('opaque-done', '*'))</script>`)}
    document.body.append(frame)
  })`)
  assert.equal(forbiddenHits, 0)
  stage = 'ordinary HTTP CORS and redirect credentials'
  await window.webContents.debugger.sendCommand('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: 0, uploadThroughput: 0 })
  assert.equal(await evaluate(`fixture.get('/fixture/redirect').then(() => true, () => false)`), false)
  assert.equal(redirectedAuthorization, '')
  assert.equal(remoteMutations, 0)
  await untrusted.loadURL(backend.replace('127.0.0.1', 'localhost') + '/page')
  assert.equal(await untrusted.webContents.executeJavaScript(`fetch(${JSON.stringify(backend + '/health')}).then(() => true, () => false)`), false)
  assert.equal(window.isVisible(), false)
  assert.equal(window.isFocused(), false)
  await writeFile(join(output, 'report.json'), JSON.stringify({ offlineChat: true, recovery: true, isolated: true, visibleWindows, focusedWindows }))
  untrusted.destroy()
  window.destroy()
  server.closeAllConnections()
  server.close()
  app.quit()
}

void run().catch((error) => {
  console.error(stage, error)
  app.exit(1)
})
