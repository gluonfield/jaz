import React, { useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { PreviewPanel } from '@/components/session/PreviewPanel'
import { SideBrowser } from '@/lib/sideBrowser'
import { BrowserProfileImport } from '@/components/browser/BrowserProfileImport'
import { exerciseProfileImport } from './profiles-ui'
import { exerciseBrowserIdentity } from './identity'
import '@/styles/globals.css'

declare global {
  interface Window {
    smoke: { backend(): Promise<string>; capture(name?: string): Promise<void>; resize(width: number, height: number): Promise<void>; result(result: unknown): void }
  }
}

function Fixture() {
  const [target, setTarget] = useState({ displayUrl: '', sourceUrl: '' })
  const [browser] = useState(() => new SideBrowser(
    (url) => setTarget({ displayUrl: url, sourceUrl: url }),
    window.jaz!.browserCommand,
    async (action, signal) => {
      const endpoint = await window.smoke.backend()
      const response = await fetch(`${endpoint}/v1/sessions/browser-fixture/browser`, { method: 'POST', body: JSON.stringify(action), signal })
      if (!response.ok) {
        throw new Error(await response.text())
      }
      return response.json()
    },
  ))
  useEffect(() => {
    const run = async () => {
      await browser.call({ method: 'Jaz.open', params: { url: `${location.origin}/target` } })
      const evaluate = async (expression: string) => {
        const result = await browser.call({ method: 'Runtime.evaluate', params: { expression, returnByValue: true, awaitPromise: true } }) as { result: { value: unknown } }
        return result.result.value
      }
      await exerciseBrowserIdentity(evaluate)
      await exerciseProfileImport(evaluate)
      const point = await evaluate(`(() => {
        const r = document.querySelector('button').getBoundingClientRect()
        return {x:r.x+r.width/2,y:r.y+r.height/2}
      })()`) as { x: number; y: number }
      const movement = browser.call({ method: 'Input.dispatchMouseEvent', params: { type: 'mouseMoved', ...point } })
      const beforeArrival = await evaluate('document.querySelector("button").matches(":hover")')
      if (beforeArrival) {
        throw new Error('Mouse input arrived before the cursor')
      }
      await movement
      await browser.call({ method: 'Input.dispatchMouseEvent', params: { type: 'mousePressed', ...point, button: 'left', clickCount: 1 } })
      await browser.call({ method: 'Input.dispatchMouseEvent', params: { type: 'mouseReleased', ...point, button: 'left', clickCount: 1 } })
      const clicked = await evaluate('window.clicks === 1 && window.trusted && document.querySelector("button").matches(":hover")')
      if (!clicked) {
        throw new Error('Expected one trusted click with hover')
      }
      await window.smoke.capture()
      const script = await browser.call({ method: 'Jaz.run', params: { code: 'const smokeCount = 7\nnodeRepl.write(smokeCount)' } }) as { text: string }
      if (!script.text.includes('7')) {
        throw new Error('Browser JavaScript runtime did not run in Electron')
      }
      const cursor = document.querySelector<HTMLElement>('[data-browser-agent-cursor]')!
      const cursorPoint = () => {
        const matrix = new DOMMatrixReadOnly(cursor.style.transform)
        return [matrix.m41, matrix.m42].join(',')
      }
      const initialCursor = cursorPoint()
      await browser.call({ method: 'Jaz.run', params: { code: `const rawCdp = tab.cdp
await rawCdp.send('Input.dispatchMouseEvent', {type:'mouseMoved',x:20,y:20,buttons:0})` } })
      if (await evaluate('document.querySelector("button").matches(":hover")') || cursorPoint() !== initialCursor) {
        throw new Error('Direct CDP must change page hover while leaving the overlay in place')
      }
      await browser.call({ method: 'Jaz.run', params: { code: `await rawCdp.send('Input.dispatchMouseEvent', ${JSON.stringify({ type: 'mouseMoved', ...point, buttons: 0 })})` } })
      if (!await evaluate('document.querySelector("button").matches(":hover")') || cursorPoint() !== initialCursor) {
        throw new Error('Persistent CDP binding did not hover the page independently')
      }
      const pendingRaw = browser.call({ method: 'Jaz.run', params: { code: `await rawCdp.send('Runtime.evaluate', {
expression:'new Promise(resolve => { window.finishRaw = resolve })',
awaitPromise:true
})` } })
      await evaluate(`new Promise(resolve => {
        const check = () => window.finishRaw ? resolve(true) : requestAnimationFrame(check)
        check()
      })`)
      browser.cancel()
      let rawCancelled = false
      try {
        await pendingRaw
      } catch {
        rawCancelled = true
      }
      await evaluate('window.finishRaw()')
      if (!rawCancelled) {
        throw new Error('A pending raw CDP script did not cancel')
      }
      const originalURL = await evaluate('location.href')
      const preparingNavigation = browser.call({ method: 'Jaz.open', params: { url: `${location.origin}/cancelled-navigation` } })
      await fetch('/wait-for-proxy')
      browser.cancel()
      await fetch('/release-proxy')
      let navigationCancelled = false
      try {
        await preparingNavigation
      } catch {
        navigationCancelled = true
      }
      if (!navigationCancelled || await evaluate('location.href') !== originalURL) {
        throw new Error('Cancelled navigation continued after its preview proxy resolved')
      }
      const cancelled = browser.call({ method: 'Input.dispatchMouseEvent', params: { type: 'mouseMoved', x: 20, y: 20 } })
      browser.cancel()
      let rejected = false
      try {
        await cancelled
      } catch {
        rejected = true
      }
      if (!rejected) {
        throw new Error('Cancelled cursor movement reached Chromium')
      }
      const tab = await browser.call({ method: 'Jaz.tab' }) as { id: string }
      let foreignRejected = false
      try {
        await window.jaz!.browserCommand({ webContentsId: Number(tab.id) - 1, method: 'Runtime.evaluate', params: { expression: '1' } })
      } catch {
        foreignRejected = true
      }
      if (!foreignRejected) {
        throw new Error('Browser command escaped its webview')
      }
      const backend = await window.smoke.backend()
      const pending = new Map<number, string>()
      const timeout = setTimeout(() => window.smoke.result({ ok: false, error: 'Browser smoke timed out', pending: [...pending.values()] }), 30000)
      const socket = new WebSocket(`${backend.replace('http:', 'ws:')}/v1/sessions/browser-fixture/browser`)
      socket.onmessage = async (event) => {
        const request = JSON.parse(event.data)
        pending.set(request.id, request.method + ' ' + String(request.params?.expression ?? '').slice(0, 100))
        try {
          const result = await browser.call(request)
          socket.send(JSON.stringify({ id: request.id, result }))
        } catch (error) {
          socket.send(JSON.stringify({ id: request.id, error: { code: -32000, message: String(error) } }))
        } finally {
          pending.delete(request.id)
        }
      }
      await new Promise((resolve, reject) => {
        socket.onopen = resolve
        socket.onerror = reject
      })
      await browser.call({ method: 'Input.dispatchMouseEvent', params: { type: 'mouseMoved', x: 20, y: 20 } })
      const beforeZeroScroll = cursorPoint()
      await browser.call({ method: 'Jaz.run', params: { code: `const targets = await tab.find('Run this step')
const targetRef = targets.elements.find(element => element.role === 'button').ref
await tab.scroll('down', 0, targetRef)` } })
      if (cursorPoint() === beforeZeroScroll || !await evaluate('scrollY === 0 && window.clicks === 1 && document.querySelector("button").matches(":hover")')) {
        throw new Error('Zero scroll must animate and hover without scrolling or clicking')
      }
      const exercise = await fetch(`${backend}/exercise?url=${encodeURIComponent(`${location.origin}/target?mcp=1`)}`)
      if (!exercise.ok) {
        throw new Error(await exercise.text())
      }
      await window.smoke.capture()
      clearTimeout(timeout)
      socket.close()
      browser.dispose()
      window.smoke.result({ ok: true, checks: ['native Chromium identity across first navigation, fetch, page, worker and client hints', 'cursor arrival precedes input', 'trusted click and hover', 'persistent direct CDP without overlay movement', 'zero scroll animates and hovers without scrolling', 'animated and raw-command cancellation', 'cancellation during preview URL resolution', 'webview ownership', 'MCP script through Go and Electron with verified page result', 'profile import rejects untrusted callers', 'selected encrypted cookies authenticate the side browser', 'import dismissal, profile selection, failure retry, themes and narrow layout'] })
    }
    void run().catch((error) => window.smoke.result({ ok: false, error: error.message, stack: error.stack }))
  }, [browser])
  return <><div id="profile-settings" className="absolute left-4 top-4"><BrowserProfileImport /></div><PreviewPanel target={target} onTargetChange={setTarget} onClose={() => {}} browserControl={browser} /></>
}

createRoot(document.getElementById('root')!).render(<Fixture />)
