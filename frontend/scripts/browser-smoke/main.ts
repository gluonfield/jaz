import { app, BrowserWindow, ipcMain, session, webContents } from 'electron'
import { createServer, type IncomingHttpHeaders, type ServerResponse } from 'node:http'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { installBrowserControl } from '@main/browserControl'
import { configurePreviewSession } from '@main/previewSession'
import { assertUntrustedProfileCaller, prepareProfileFixture } from './profiles'
import { accessibilityFixture, accessibilityFrame } from './accessibility'

app.setName('Jaz')
app.setPath('userData', join(process.env.JAZ_BROWSER_SMOKE_DIR!, `profile-${process.pid}`))
const timeout = Number(process.env.JAZ_BROWSER_SMOKE_TIMEOUT_MS || 30000)
installBrowserControl()
process.on('unhandledRejection', (error) => {
  console.error(error)
  app.exit(1)
})
ipcMain.handle('smoke:backend', () => process.env.JAZ_BROWSER_SMOKE_BACKEND)
ipcMain.handle('smoke:browser-exists', (_event, id: number) => Boolean(webContents.fromId(id)))

let pendingProxy: { response: ServerResponse; url: string } | undefined
let proxyWaiter: ServerResponse | undefined
let firstNavigation: IncomingHttpHeaders | undefined
const server = createServer(async (request, response) => {
  if (request.url?.startsWith('/accessibility')) {
    response.setHeader('Content-Type', 'text/html')
    response.end(request.url.startsWith('/accessibility-frame')
      ? accessibilityFrame(new URL(request.url, `http://${request.headers.host}`))
      : accessibilityFixture(`http://${request.headers.host}`))
    return
  }
  if (request.url === '/target' && !firstNavigation) {
    firstNavigation = request.headers
  }
  if (request.url === '/browser-identity') {
    response.setHeader('Content-Type', 'application/json')
    response.end(JSON.stringify({ firstNavigation, request: request.headers, chromeVersion: process.versions.chrome, shellUserAgent: session.defaultSession.getUserAgent() }))
    return
  }
  if (request.url === '/profile-session') {
    response.end(request.headers.cookie?.includes('jaz_import_fixture=signed-in-fixture') ? 'Imported session is active' : 'Not signed in')
    return
  }
  if (request.url === '/wait-for-proxy') {
    if (pendingProxy) {
      response.end()
    } else {
      proxyWaiter = response
    }
    return
  }
  if (request.url === '/release-proxy') {
    pendingProxy!.response.end(JSON.stringify({ url: pendingProxy!.url }))
    pendingProxy = undefined
    response.end()
    return
  }
  if (request.url === '/v1/preview/proxies') {
    let body = ''
    for await (const chunk of request) {
      body += chunk
    }
    response.setHeader('Content-Type', 'application/json')
    const url = JSON.parse(body).url as string
    if (new URL(url).pathname === '/cancelled-navigation') {
      pendingProxy = { response, url }
      proxyWaiter?.end()
      proxyWaiter = undefined
      return
    }
    response.end(JSON.stringify({ url }))
    return
  }
  const pathname = new URL(request.url!, 'http://localhost').pathname
  const name = pathname === '/target' ? 'target.html' : /\.(m?js|wasm|css|woff2?)$/.test(pathname) ? pathname.slice(1) : 'index.html'
  response.setHeader('Content-Type', /\.m?js$/.test(name) ? 'text/javascript' : name.endsWith('.wasm') ? 'application/wasm' : name.endsWith('.css') ? 'text/css' : name.endsWith('.woff2') ? 'font/woff2' : 'text/html')
  response.end(await readFile(join(process.env.JAZ_BROWSER_SMOKE_DIR!, name)))
})
server.listen(0, '127.0.0.1', async () => {
  const address = server.address()
  if (!address || typeof address === 'string') {
    throw new Error('Missing test server address')
  }
  await app.whenReady()
  configurePreviewSession()
  await prepareProfileFixture(process.env.JAZ_BROWSER_SMOKE_DIR!)
  process.env.ELECTRON_RENDERER_URL = `http://127.0.0.1:${address.port}`
  const window = new BrowserWindow({
    width: 1050,
    height: 850,
    webPreferences: {
      webviewTag: true,
      contextIsolation: true,
      sandbox: true,
      preload: join(process.env.JAZ_BROWSER_SMOKE_DIR!, 'preload.js'),
    },
  })
  window.webContents.on('console-message', ({ level, message }) => {
    if (level === 'warning' || level === 'error') {
      console.error(message)
    }
  })
  await assertUntrustedProfileCaller(process.env.JAZ_BROWSER_SMOKE_DIR!)
  ipcMain.handle('smoke:capture', async (_event, name = 'browser') => {
    if (!/^[a-z-]+$/.test(name)) {
      throw new Error('Invalid screenshot name')
    }
    const screenshot = await window.webContents.capturePage()
    await writeFile(join(process.env.JAZ_BROWSER_SMOKE_DIR!, name + '.png'), screenshot.toPNG())
  })
  ipcMain.handle('smoke:resize', (_event, width: number, height: number) => {
    window.setContentSize(width, height)
  })
  ipcMain.on('smoke:result', (_event, result) => {
    console.log(JSON.stringify(result))
    server.close()
    app.exit(result.ok ? 0 : 1)
  })
  await window.loadURL(`http://127.0.0.1:${address.port}?timeout=${timeout}`)
})
setTimeout(() => app.exit(2), timeout + 10000)
