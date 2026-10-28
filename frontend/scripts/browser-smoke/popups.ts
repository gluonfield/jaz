import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { BrowserWindow, session, type WebContents } from 'electron'
import { attachPreviewWebviews } from '@main/previewSession'
import { PREVIEW_PARTITION } from '@shared/preview'

async function until(check: () => boolean | Promise<boolean>): Promise<void> {
  const deadline = Date.now() + 3000
  while (!await check()) {
    assert(Date.now() < deadline, 'Popup did not complete')
    await new Promise(resolve => setTimeout(resolve, 20))
  }
}

export async function exerciseBrowserPopups(preload: string, openedURLs: string[]): Promise<void> {
  let origin = ''
  let posted = ''
  const server = createServer(async (request, response) => {
    const url = new URL(request.url!, origin)
    response.setHeader('Content-Type', 'text/html')
    if (url.pathname === '/host') {
      response.end(`<webview src="${origin}/parent" partition="${PREVIEW_PARTITION}" allowpopups style="width:800px;height:600px"></webview>`)
    } else if (url.pathname === '/parent') {
      response.end(`<script>
        window.received = []
        addEventListener('message', event => {
          if (event.origin === '${origin.replace('127.0.0.1', 'localhost')}') {
            window.received.push(event.data)
          }
        })
      </script>`)
    } else if (url.pathname === '/redirect') {
      response.writeHead(302, { Location: '/child' })
      response.end()
    } else if (url.pathname === '/post') {
      for await (const chunk of request) {
        posted += chunk
      }
      response.writeHead(303, { Location: '/child' })
      response.end()
    } else if (url.pathname === '/blocked-redirect') {
      response.writeHead(302, { Location: 'file:///tmp/jaz-popup-must-not-load.html' })
      response.end()
    } else {
      if (url.pathname === '/isolated') {
        response.setHeader('Cross-Origin-Opener-Policy', 'same-origin')
      }
      response.end(`<title>Untrusted page title</title><h1>Popup fixture</h1><script>
        window.signedIn = ${request.headers.cookie?.split('; ').includes('popup_fixture=active') ?? false}
        if (window.opener) {
          window.opener.postMessage('complete', '${origin}')
        }
      </script>`)
    }
  })
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  assert(address && typeof address !== 'string')
  origin = `http://127.0.0.1:${address.port}`
  const childOrigin = origin.replace('127.0.0.1', 'localhost')
  await session.fromPartition(PREVIEW_PARTITION).cookies.set({ url: childOrigin, name: 'popup_fixture', value: 'active' })
  const host = new BrowserWindow({ show: false, webPreferences: { webviewTag: true, contextIsolation: true, nodeIntegration: false, sandbox: true } })
  attachPreviewWebviews(host.webContents, preload)
  const attached = once(host.webContents, 'did-attach-webview')
  try {
    await host.loadURL(origin + '/host')
    const [, parent] = await attached as [unknown, WebContents]
    await until(async () => await parent.executeJavaScript('Array.isArray(window.received)'))
    const externalURL = origin + '/app-link'
    assert.equal(await host.webContents.executeJavaScript(`window.open(${JSON.stringify(externalURL)}) === null`, true), true)
    assert.equal(openedURLs.at(-1), externalURL)
    const externalBefore = openedURLs.length
    const open = async (owner: WebContents, url: string, features = 'width=520,height=600') => {
      const created = once(owner, 'did-create-window', { signal: AbortSignal.timeout(3000) })
      const returnedNull = await owner.executeJavaScript(`window.popup = window.open(${JSON.stringify(url)}, '_blank', ${JSON.stringify(features)})
        window.popup === null`, true)
      const [popup] = await created as [BrowserWindow]
      assert.equal(returnedNull, features.includes('noopener'))
      await until(() => !popup.webContents.isLoading())
      assert.equal(popup.webContents.session, parent.session)
      const preferences = popup.webContents.getLastWebPreferences()
      assert.equal(preferences.nodeIntegration, false)
      assert.equal(preferences.nodeIntegrationInSubFrames, false)
      assert.equal(preferences.contextIsolation, true)
      assert.equal(preferences.sandbox, true)
      assert.equal(preferences.webSecurity, true)
      assert.equal(preferences.allowRunningInsecureContent, false)
      assert.equal(preferences.webviewTag, false)
      assert.deepEqual(await popup.webContents.executeJavaScript('[typeof require, typeof process, typeof window.jaz, chrome.app.runningState()]'), ['undefined', 'undefined', 'undefined', 'cannot_run'])
      return popup
    }
    const callback = await open(parent, childOrigin + '/redirect', 'width=520,height=600,nodeIntegration=yes,contextIsolation=no,webviewTag=yes,frame=no')
    await until(async () => await parent.executeJavaScript('window.received.length') === 1)
    assert.equal(await callback.webContents.executeJavaScript('Boolean(window.opener) && window.signedIn'), true)
    assert.equal(callback.getTitle(), childOrigin)
    await callback.webContents.executeJavaScript('document.title = "Trusted bank"')
    await until(() => callback.webContents.getTitle() === 'Trusted bank')
    assert.equal(callback.getTitle(), childOrigin)
    await callback.webContents.executeJavaScript('window.close()')
    await until(() => callback.isDestroyed())
    assert.equal(await parent.executeJavaScript('window.popup.closed'), true)

    const empty = await open(parent, '')
    assert.equal(await empty.webContents.executeJavaScript('Boolean(window.opener)'), true)
    empty.destroy()
    const blank = await open(parent, 'about:blank')
    assert.equal(await blank.webContents.executeJavaScript('Boolean(window.opener)'), true)
    await blank.webContents.executeJavaScript(`location.href = ${JSON.stringify(childOrigin + '/redirect')}`)
    await until(async () => await parent.executeJavaScript('window.received.length') === 2)
    const nested = await open(blank.webContents, childOrigin + '/child')
    assert.equal(await nested.webContents.executeJavaScript('Boolean(window.opener)'), true)

    const noOpener = await open(parent, childOrigin + '/child', 'noopener,width=520,height=600')
    assert.equal(await noOpener.webContents.executeJavaScript('window.opener === null'), true)
    noOpener.destroy()
    const isolated = await open(parent, childOrigin + '/isolated')
    assert.equal(await isolated.webContents.executeJavaScript('window.opener === null'), true)
    isolated.destroy()

    const postedWindow = once(parent, 'did-create-window')
    await parent.executeJavaScript(`(() => {
      const form = document.createElement('form')
      form.method = 'POST'
      form.target = '_blank'
      form.action = ${JSON.stringify(childOrigin + '/post')}
      const field = document.createElement('input')
      field.name = 'fixture'
      field.value = 'kept'
      form.append(field)
      document.body.append(form)
      form.submit()
    })()`, true)
    const [postPopup] = await postedWindow as [BrowserWindow]
    await until(() => posted === 'fixture=kept' && postPopup.webContents.getURL() === childOrigin + '/child')
    postPopup.destroy()

    const windowCount = BrowserWindow.getAllWindows().length
    for (const url of ['file:///tmp/jaz-popup-must-not-load.html', 'data:text/html,blocked']) {
      assert.equal(await parent.executeJavaScript(`window.open(${JSON.stringify(url)}) === null`, true), true)
    }
    assert.equal(BrowserWindow.getAllWindows().length, windowCount)
    const failedRedirect = once(blank.webContents, 'did-fail-load', { signal: AbortSignal.timeout(3000) })
    await blank.webContents.executeJavaScript(`location.href = ${JSON.stringify(childOrigin + '/blocked-redirect')}`)
    const [, , failure] = await failedRedirect
    assert.equal(failure, 'ERR_UNSAFE_REDIRECT')
    assert.equal(new URL(blank.webContents.getURL()).origin, childOrigin)
    assert.equal(parent.getURL(), origin + '/parent')
    assert.equal(openedURLs.length, externalBefore)
    host.destroy()
    await until(() => blank.isDestroyed() && nested.isDestroyed())
    console.log('Browser popup opener, shared session, redirects, blank/nested windows, POST, close and security boundaries passed')
  } finally {
    if (!host.isDestroyed()) {
      host.destroy()
    }
    await new Promise<void>(resolve => server.close(() => resolve()))
  }
}
