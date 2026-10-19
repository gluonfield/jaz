import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { writeFile } from 'node:fs/promises'
import { createInterface } from 'node:readline'
import { join } from 'node:path'
import { BrowserWindow, clipboard, webContents, type WebContents, type WebFrameMain } from 'electron'
import { PREVIEW_PARTITION } from '@shared/preview'

export function clipboardFixture(url: URL): string {
  const buttons = ['copy-id', 'copy-link', 'read', 'media'].map(action => `<button data-action="${action}">${action}</button>`).join('')
  const document = `<style>body{margin:0}button{display:block;width:180px;height:40px}</style>${buttons}<script>
document.addEventListener('click', async event => {
  const action = event.target.dataset.action
  if (!action) {
    return
  }
  const activated = event.isTrusted && navigator.userActivation.isActive
  let ok = true
  let error = ''
  try {
    if (action.startsWith('copy-')) {
      await navigator.clipboard.writeText(action === 'copy-id' ? 'JAZ-123' : 'https://tasks.jaz.chat/issue/JAZ-123')
    }
    if (action === 'read') {
      await navigator.clipboard.readText()
    }
    if (action === 'media') {
      const stream = await navigator.mediaDevices.getUserMedia({audio:true})
      stream.getTracks().forEach(track => track.stop())
    }
  } catch (failure) {
    ok = false
    error = failure.name
  }
  parent.postMessage({clipboardResult:{action,ok,error,activated}}, '*')
})
addEventListener('message', event => {
  if (event.data.clipboardResult) {
    window.clipboardResult = event.data.clipboardResult
  }
})
</script>`
  if (url.searchParams.has('child')) {
    return document
  }
  const source = url.searchParams.has('delegated')
    ? `src="${url.origin}/clipboard?child"`
    : `srcdoc="${document.replaceAll('&', '&amp;').replaceAll('"', '&quot;')}"`
  const sandbox = url.searchParams.has('delegated') ? 'allow-scripts allow-same-origin' : 'allow-scripts allow-forms allow-popups'
  const allow = url.searchParams.has('delegated') ? 'clipboard-write; clipboard-read; microphone; local-fonts' : 'clipboard-write'
  return `${document}<iframe style="position:absolute;left:220px;top:0;width:500px;height:400px;border:0" sandbox="${sandbox}" allow="${allow}" ${source}></iframe>`
}

async function preserveClipboard(): Promise<() => Promise<void>> {
  if (process.platform === 'darwin') {
    const guard = spawn('/usr/bin/swift', [join(process.env.JAZ_BROWSER_SMOKE_DIR!, 'clipboard-guard.swift')], { stdio: ['pipe', 'pipe', 'inherit'] })
    const lines = createInterface({ input: guard.stdout })[Symbol.asyncIterator]()
    const exited = once(guard, 'exit')
    assert.equal((await lines.next()).value, 'saved')
    return async () => {
      guard.stdin.end('\n')
      assert.equal((await lines.next()).value, 'restored')
      assert.deepEqual(await exited, [0, null])
    }
  }
  const formats = clipboard.availableFormats()
  assert(formats.every(format => ['text/plain', 'text/html', 'text/rtf', 'image/png'].includes(format)), 'Cannot safely preserve this clipboard format')
  const buffers = formats.map(format => [format, clipboard.readBuffer(format)] as const)
  const saved = {
    text: formats.includes('text/plain') ? clipboard.readText() : undefined,
    html: formats.includes('text/html') ? clipboard.readHTML() : undefined,
    rtf: formats.includes('text/rtf') ? clipboard.readRTF() : undefined,
    image: formats.includes('image/png') ? clipboard.readImage() : undefined,
  }
  return async () => {
    clipboard.clear()
    if (formats.length) {
      clipboard.write(saved)
    }
    assert(buffers.every(([format, data]) => clipboard.readBuffer(format).equals(data)), 'Clipboard restoration must preserve every saved format')
  }
}

async function click(contents: WebContents, frame: WebFrameMain, action: string, x = 0) {
  const restore = await preserveClipboard()
  try {
    await contents.executeJavaScript('window.clipboardResult = undefined')
    const point = await frame.executeJavaScript(`(() => {
      const button = document.querySelector('[data-action="${action}"]')
      button.focus()
      const rect = button.getBoundingClientRect()
      return {x:rect.x + rect.width / 2,y:rect.y + rect.height / 2}
    })()`)
    for (const type of ['mousePressed', 'mouseReleased']) {
      await contents.debugger.sendCommand('Input.dispatchMouseEvent', { type, x: point.x + x, y: point.y, button: 'left', clickCount: 1 })
    }
    for (let attempt = 0; attempt < 100; attempt++) {
      const result = await contents.executeJavaScript('window.clipboardResult')
      if (result) {
        assert.equal(result.action, action)
        assert.equal(result.activated, true, 'Clipboard action must come from a real activated click')
        if (result.ok && action.startsWith('copy-')) {
          assert(clipboard.readText() === (action === 'copy-id' ? 'JAZ-123' : 'https://tasks.jaz.chat/issue/JAZ-123'), 'Clipboard must contain the clicked value')
        }
        return result
      }
      await new Promise(resolve => setTimeout(resolve, 20))
    }
    throw new Error(`Clipboard action timed out: ${action}`)
  } finally {
    await restore()
  }
}

async function focus(contents: WebContents) {
  contents.debugger.attach('1.3')
  await contents.debugger.sendCommand('Emulation.setFocusEmulationEnabled', { enabled: true })
}

export async function exerciseClipboard(origin: string): Promise<void> {
  console.log(JSON.stringify({ stage: 'clipboard permissions', pid: process.pid }))
  const window = new BrowserWindow({ show: false, webPreferences: { sandbox: true, contextIsolation: true, backgroundThrottling: false, webviewTag: true } })
  try {
    await window.loadURL(origin + '/clipboard')
    await focus(window.webContents)
    assert.equal(await window.webContents.mainFrame.frames[0].executeJavaScript("navigator.permissions.query({name:'clipboard-write'}).then(result => result.state)"), 'granted', 'Embedded write permission checks must succeed')
    for (const action of ['copy-id', 'copy-link']) {
      assert.equal((await click(window.webContents, window.webContents.mainFrame.frames[0], action, 220)).ok, true, 'Sandboxed embedded app copy must succeed')
    }
    assert.equal((await click(window.webContents, window.webContents.mainFrame, 'copy-id')).ok, true, 'Trusted renderer copy must succeed')
    const file = join(process.env.JAZ_BROWSER_SMOKE_DIR!, 'renderer', 'index.html')
    const rendererURL = process.env.ELECTRON_RENDERER_URL
    delete process.env.ELECTRON_RENDERER_URL
    try {
      await writeFile(file, clipboardFixture(new URL('http://localhost/clipboard')))
      await window.loadFile(file)
      assert.equal((await click(window.webContents, window.webContents.mainFrame.frames[0], 'copy-link', 220)).ok, true, 'Packaged embedded app copy must succeed')
    } finally {
      process.env.ELECTRON_RENDERER_URL = rendererURL
    }
    await window.loadURL(origin + '/clipboard?delegated')
    for (const permission of ['clipboard-read', 'microphone', 'local-fonts']) {
      assert.equal(await window.webContents.mainFrame.frames[0].executeJavaScript(`navigator.permissions.query({name:'${permission}'}).then(result => result.state)`), 'denied', `Embedded ${permission} permission checks must stay denied`)
    }
    for (const action of ['read', 'media']) {
      const result = await click(window.webContents, window.webContents.mainFrame.frames[0], action, 220)
      assert.equal(result.ok, false, `Embedded ${action} must stay denied even with iframe delegation`)
      assert.equal(result.error, 'NotAllowedError')
    }
    assert.equal((await click(window.webContents, window.webContents.mainFrame, 'read')).ok, false, 'Trusted renderer cannot read the clipboard')
    await window.loadURL(origin.replace('127.0.0.1', 'localhost') + '/clipboard')
    for (const frame of window.webContents.mainFrame.framesInSubtree) {
      assert.equal((await click(window.webContents, frame, 'copy-id', frame === window.webContents.mainFrame ? 0 : 220)).ok, false, 'Untrusted windows cannot copy')
    }
    await window.loadURL(origin + '/clipboard?child')
    await window.webContents.executeJavaScript(`document.body.innerHTML = '<webview style="width:700px;height:400px" partition="${PREVIEW_PARTITION}" src="${origin}/clipboard?child"></webview>'`)
    let preview: WebContents | undefined
    for (let attempt = 0; attempt < 100; attempt++) {
      preview = webContents.getAllWebContents().find(contents => contents.hostWebContents === window.webContents)
      if (preview && !preview.isLoading()) {
        break
      }
      await new Promise(resolve => setTimeout(resolve, 20))
    }
    assert(preview && !preview.isLoading(), 'Side browser must load its real webview')
    await focus(preview)
    assert.equal((await click(preview, preview.mainFrame, 'copy-link')).ok, true, 'Side browser copy must remain available')
  } finally {
    window.destroy()
  }
}
