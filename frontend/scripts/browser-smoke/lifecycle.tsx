import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useCallback, useLayoutEffect, useState, type CSSProperties } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserPanelSlot, BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanelResizeHandle } from '@/components/session/SidePanelResizeHandle'
import { setApiBaseUrl } from '@/lib/api/client'
import { useSessionPreview } from '@/lib/browserSessions'
import type { PreviewWebviewElement } from '@/components/session/previewWebview'
import type { BrowserActionResult } from '@/lib/browserApi'

export async function exerciseBrowserLifecycle(backend: string, onStage: (stage: string) => void): Promise<void> {
  const fetchRequest = window.fetch
  setApiBaseUrl(backend)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const element = document.createElement('div')
  element.style.cssText = 'position:absolute;inset:0;background:white'
  document.body.append(element)
  const root = createRoot(element)
  let selectChat: (id: string) => void
  let showPanel: (visible: boolean) => void
  function Chat({ id }: { id: string }) {
    const [visible, setVisible] = useState(true)
    const [width, setWidth] = useState(640)
    const show = useCallback(() => setVisible(true), [])
    const close = useCallback(() => setVisible(false), [])
    const annotate = useCallback(() => {
      throw new Error('An abandoned annotation was submitted')
    }, [])
    useSessionPreview(id, show)
    useLayoutEffect(() => {
      showPanel = setVisible
    }, [])
    return <div style={{ position: 'absolute', top: 40, right: 0, width, bottom: 0, '--side-panel-width': `${width}px` } as CSSProperties}>
      <SidePanelResizeHandle width={width} minWidth={640} maxWidth={800} disabled={!visible} onResize={setWidth} onResizeStart={() => {}} onResizeEnd={() => {}} />
      {visible ? <BrowserPanelSlot sessionId={id} visible onClose={close} onAddBrowserAnnotation={annotate} /> : <div>Another panel</div>}
    </div>
  }
  function Fixture() {
    const [id, setId] = useState('browser-fixture')
    useLayoutEffect(() => {
      selectChat = setId
    }, [])
    return <QueryClientProvider client={queryClient}><BrowserWorkspace idleMs={100}><Chat key={id} id={id} /></BrowserWorkspace></QueryClientProvider>
  }
  const waitFor = async (check: () => boolean | Promise<boolean>, timeoutMs = 5000) => {
    const end = Date.now() + timeoutMs
    while (!await check()) {
      if (Date.now() > end) throw new Error('Browser lifecycle condition timed out')
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const script = async (id: string, code: string): Promise<BrowserActionResult> => {
    const response = await fetch(`${backend}/exercise-script/${id}`, { method: 'POST', body: JSON.stringify({ code }) })
    if (!response.ok) throw new Error(await response.text())
    return response.json()
  }
  const panel = (id: string) => document.querySelector<HTMLElement>(`[data-browser-session="${id}"]`)!
  const webview = (id: string) => panel(id).querySelector('webview') as PreviewWebviewElement
  const evaluate = async (view: PreviewWebviewElement, expression: string) => {
    const result = await window.jaz!.browserCommand({ webContentsId: view.getWebContentsId(), method: 'Runtime.evaluate', params: { expression, returnByValue: true } }) as { result: { value: unknown } }
    return result.result.value
  }
  const connected = async (id: string) => {
    await waitFor(async () => {
      const response = await fetch(`${backend}/v1/sessions/${id}/browser`, { method: 'POST', body: '{"action":"status"}' })
      return response.ok
    })
  }
  const status = async (id: string, state: string, queued = false) => {
    const response = await fetch(`${backend}/exercise-status/${id}`, { method: 'POST', body: JSON.stringify({ status: state, queued }) })
    if (!response.ok) throw new Error(await response.text())
  }
  const beyondIdle = () => new Promise((resolve) => setTimeout(resolve, 250))
  try {
    await status('browser-fixture', 'running')
    await status('browser-background', 'running')
    root.render(<Fixture />)
    await connected('browser-fixture')
    await script('browser-fixture', `const retained = 41
await tab.goto(${JSON.stringify(`${location.origin}/target?lifecycle=a`)})
const tree = await tab.getAXState()
const target = Number(tree.match(/([0-9]+) button "Run this step"/)[1])`)
    const firstView = webview('browser-fixture')
    const firstID = firstView.getWebContentsId()
    const firstSize = await evaluate(firstView, '[innerWidth,innerHeight].join(",")')
    const inFlight = script('browser-fixture', `await tab.cdp.send('Runtime.evaluate', {
expression: 'new Promise(resolve => { window.finishBackground = resolve })',
awaitPromise: true
})
await tab.click(target)
await tab.getAXState()
nodeRepl.write(retained)`)
    await waitFor(async () => await evaluate(firstView, 'typeof window.finishBackground') === 'function')
    selectChat!('browser-background')
    await connected('browser-background')
    await waitFor(() => panel('browser-fixture').inert)
    await evaluate(firstView, 'window.finishBackground()')
    const finished = await inFlight
    if (!finished.text?.includes('41') || !await evaluate(firstView, 'window.clicks === 1 && window.trusted')) {
      throw new Error('An in-flight script did not continue with trusted input after switching chats')
    }
    const hiddenSize = await evaluate(firstView, '[innerWidth,innerHeight].join(",")')
    if (hiddenSize !== firstSize) {
      throw new Error(`Switching chats changed the hidden browser viewport: ${firstSize} -> ${hiddenSize}`)
    }
    const screenshot = await script('browser-fixture', 'await tab.getScreenshot()')
    if (!screenshot.image_base64) throw new Error('Hidden browser screenshot was not captured')
    selectChat!('browser-fixture')
    await waitFor(() => !panel('browser-fixture').inert)
    if (webview('browser-fixture').getWebContentsId() !== firstID || !await evaluate(firstView, 'window.clicks === 1')) {
      throw new Error('Returning to a chat replaced or reloaded its browser')
    }
    onStage('script-only browser cleanup')
    await script('browser-background', 'const idleScratch = 41')
    await status('browser-background', 'idle')
    await beyondIdle()
    if (webview('browser-background')) throw new Error('A script-only browser created a webview')
    await status('browser-background', 'running')
    const emptyResume = await script('browser-background', 'nodeRepl.write(typeof idleScratch)')
    if (!emptyResume.text?.endsWith('undefined')) throw new Error('The idle script-only browser retained its JavaScript context')
    onStage('background browser lifecycle')
    await script('browser-background', `const retained = 92
await tab.goto(${JSON.stringify(`${location.origin}/target?lifecycle=b`)})
const tree = await tab.getAXState()
await tab.click(Number(tree.match(/([0-9]+) button "Run this step"/)[1]))
await tab.getAXState()`)
    if (!panel('browser-background').inert || !await evaluate(webview('browser-background'), 'window.clicks === 1 && window.trusted')) {
      throw new Error('A background chat could not open and control its browser independently')
    }
    panel('browser-fixture').querySelector<HTMLButtonElement>('[aria-label="Annotate preview"]')!.click()
    await waitFor(async () => await evaluate(firstView, 'typeof window.__jazAnnotationCancel') === 'function')
    showPanel!(false)
    await waitFor(() => panel('browser-fixture').inert)
    await script('browser-fixture', 'await tab.click(target)\nawait tab.getAXState()')
    if (!await evaluate(firstView, 'window.clicks === 2')) throw new Error('Switching panels stopped the browser')
    showPanel!(true)
    await waitFor(() => !panel('browser-fixture').inert)
    const first = await script('browser-fixture', 'nodeRepl.write(retained)')
    const second = await script('browser-background', 'nodeRepl.write(retained)')
    if (first.text !== '41' || second.text !== '92') throw new Error('Browser script bindings crossed conversations')
    const bounds = panel('browser-fixture').getBoundingClientRect()
    const anchor = element.querySelector<HTMLElement>('[style*="anchor-name"]')!.getBoundingClientRect()
    if (Math.abs(bounds.x - anchor.x) > 1 || Math.abs(bounds.y - anchor.y) > 1 || Math.abs(bounds.width - anchor.width) > 1) {
      throw new Error('Restored browser no longer aligns with its panel')
    }
    const resize = element.querySelector<HTMLElement>('[role="separator"]')!
    const grip = resize.getBoundingClientRect()
    for (const x of [grip.left + 2, grip.right - 2]) {
      if (!resize.contains(document.elementFromPoint(x, grip.top + 100))) {
        throw new Error('The retained browser covers its resize handle')
      }
    }
    resize.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }))
    await waitFor(() => panel('browser-fixture').getBoundingClientRect().width === 664)
    await window.smoke.capture('browser-lifecycle')
    onStage('browser idle release')
    selectChat!('browser-background')
    await waitFor(() => panel('browser-fixture').inert)
    await beyondIdle()
    if (!await window.smoke.browserExists(firstID)) throw new Error('Browser was released while its agent was working')
    await status('browser-fixture', 'idle', true)
    await beyondIdle()
    if (!await window.smoke.browserExists(firstID)) throw new Error('Browser was released with queued agent work')
    const pending = script('browser-fixture', `await tab.cdp.send('Runtime.evaluate', {
expression: 'new Promise(resolve => { window.finishRetention = resolve })',
awaitPromise: true
})`)
    await waitFor(async () => await evaluate(firstView, 'typeof window.finishRetention') === 'function')
    await status('browser-fixture', 'idle')
    await beyondIdle()
    if (!await window.smoke.browserExists(firstID)) throw new Error('Browser was released during a pending command')
    await evaluate(firstView, 'window.finishRetention()')
    await pending
    await waitFor(async () => !webview('browser-fixture') && !await window.smoke.browserExists(firstID))
    onStage('browser background resume')
    const restored = await script('browser-fixture', `nodeRepl.write(typeof retained)
const restoredTree = await tab.getAXState()
await tab.click(Number(restoredTree.match(/([0-9]+) button "Run this step"/)[1]))
await tab.getAXState()`)
    const restoredView = webview('browser-fixture')
    const restoredID = restoredView.getWebContentsId()
    if (restoredID === firstID || !restored.text?.includes('undefined') || !panel('browser-fixture').inert || !await evaluate(restoredView, 'window.clicks === 1 && window.trusted')) {
      throw new Error('An unloaded browser did not resume independently with a fresh page and script scope')
    }
    selectChat!('browser-fixture')
    await waitFor(() => !panel('browser-fixture').inert)
    await beyondIdle()
    if (!await window.smoke.browserExists(restoredID)) throw new Error('A visible idle browser was released')
    onStage('stalled browser status')
    let stalledStatus = false
    window.fetch = (input, init) => {
      if (stalledStatus || input !== `${backend}/v1/sessions/browser-fixture`) return fetchRequest(input, init)
      stalledStatus = true
      const signal = init?.signal
      return new Promise((_resolve, reject) => {
        signal?.addEventListener('abort', () => reject(signal.reason), { once: true })
      })
    }
    showPanel!(false)
    await waitFor(async () => !await window.smoke.browserExists(restoredID), 15_000)
    window.fetch = fetchRequest
    if (!stalledStatus) throw new Error('The stalled status check was not exercised')
    showPanel!(true)
    await waitFor(() => Boolean(webview('browser-fixture')))
    await script('browser-fixture', 'await tab.getAXState()')
    if (webview('browser-fixture').getWebContentsId() === restoredID) throw new Error('Opening Preview did not recreate the unloaded page')
  } finally {
    window.fetch = fetchRequest
    root.unmount()
    queryClient.clear()
    element.remove()
    setApiBaseUrl(location.origin)
  }
}
