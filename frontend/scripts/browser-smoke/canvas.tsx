import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { ThreadCanvas } from '@/components/canvas/ThreadCanvas'
import { applyPatch } from '@/components/canvas/board'
import { useCanvasContexts } from '@/components/canvas/useCanvasContexts'
import type { CanvasView } from '@/lib/api/canvas'
import { keys } from '@/lib/query/keys'

export async function exerciseCanvas(): Promise<void> {
  const sessionId = 'canvas-smoke'
  const linkURL = new URL('/canvas-link', location.origin)
  linkURL.hostname = 'localhost'
  const paragraphs = Array.from({ length: 8 }, (_, i) => `Paragraph ${i + 1}: This entire document fits inside its canvas item.`).join('\n\n')
  let view: CanvasView = {
    version: '1', dir: '/fixture', edges: [], components: {},
    nodes: [
      { id: 'report', type: 'file', file: 'report.md', x: 0, y: 0, width: 320, height: 260 },
      { id: 'table', type: 'file', file: 'costs.csv', x: 480, y: 0, width: 320, height: 260 },
      { id: 'page', type: 'link', url: linkURL.href, x: 0, y: 440, width: 180, height: 160 },
      { id: 'sticky', type: 'text', text: paragraphs, color: '3', x: 360, y: 440, width: 200, height: 160 },
      { id: 'text', type: 'text', text: paragraphs, x: 600, y: 440, width: 200, height: 160 },
      { id: 'cad', type: 'file', file: 'broken.stl', x: 480, y: 300, width: 320, height: 100 },
      { id: 'artifact', type: 'file', file: 'card.html', x: 840, y: 0, width: 320, height: 260 },
    ],
    files: { report: { path: 'report.md', version: '1' }, table: { path: 'costs.csv', version: '1' }, cad: { path: 'broken.stl', version: '1' }, artifact: { path: 'card.html', version: '1' } },
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(keys.sessionCanvas(sessionId), view)
  client.setQueryData([...keys.sessionFile(sessionId, 'report.md'), '1'], { content: paragraphs })
  const csv = Array.from({ length: 40 }, (_, i) => Array.from({ length: 12 }, (_, column) => `Part-${i}-${column}`).join(',')).join('\n')
  const header = Array.from({ length: 12 }, (_, column) => `Column-${column}`).join(',')
  client.setQueryData([...keys.sessionFile(sessionId, 'costs.csv'), '1'], { content: header + '\n' + csv })
  client.setQueryData([...keys.sessionFile(sessionId, 'card.html'), '1'], { content: `
<button id="more">More</button><main>${Array.from({ length: 12 }, (_, i) => `<p>Card row ${i}</p>`).join('')}</main>
<script>
let clicks = 0
const report = () => requestAnimationFrame(() => requestAnimationFrame(() => {
  const last = document.querySelector('main').lastElementChild.getBoundingClientRect()
  const button = document.querySelector('button').getBoundingClientRect()
  parent.postMessage({ type: 'fixture:layout', clicks, bottom: last.bottom, height: innerHeight, button: { x: button.x, y: button.y, width: button.width, height: button.height } }, '*')
}))
window.addEventListener('load', report)
window.addEventListener('resize', report)
document.querySelector('button').onclick = () => {
  clicks++
  document.querySelector('main').insertAdjacentHTML('beforeend', '<p>Added row</p>')
  report()
}
</script>` })
  let artifactLayout: { clicks: number; bottom: number; height: number; button: { x: number; y: number; width: number; height: number } } | undefined
  let linkIdentity: { request: Record<string, string>; referrer: string; userAgent: string } | undefined
  const onArtifactMessage = (event: MessageEvent) => {
    if (event.source === node('artifact')?.querySelector('iframe')?.contentWindow && event.data?.type === 'fixture:layout') {
      artifactLayout = event.data
    }
    if (event.source === node('page')?.querySelector('iframe')?.contentWindow && event.data?.type === 'fixture:canvas-link') {
      linkIdentity = event.data
    }
  }
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const url = new URL(String(input))
    if (url.pathname === `/v1/sessions/${sessionId}/file` && url.searchParams.get('path') === 'broken.stl') {
      return new Response('Invalid STL')
    }
    if (url.pathname !== `/v1/sessions/${sessionId}/canvas`) {
      return originalFetch(input, init)
    }
    if (init?.method === 'PATCH') {
      view = { ...applyPatch(view, JSON.parse(String(init.body))), version: String(Number(view.version) + 1) }
      return Response.json({ version: view.version })
    }
    return Response.json(view)
  }
  function Fixture({ inset = 0 }: { inset?: number }) {
    const { selection, onSelect } = useCanvasContexts({ contexts: [], removeContext: () => {}, replaceContexts: () => {} })
    return <ThreadCanvas sessionId={sessionId} inset={inset} working={false} selection={selection} onSelect={onSelect} onPrompt={() => {}} />
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const until = async (check: () => boolean, message = check.toString()) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Canvas: ' + message)
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const node = (id: string) => element.querySelector<HTMLElement>(`.react-flow__node[data-id="${id}"]`)!
  const viewport = () => element.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform
  const zoom = () => Number(viewport().match(/scale\(([^)]+)\)/)![1])
  const fits = (id: string) => {
    const fitted = node(id).querySelector<HTMLElement>('.canvas-fit')!
    const last = [...fitted.querySelectorAll(id === 'table' ? 'td' : 'p')].at(-1)!.getBoundingClientRect()
    const box = fitted.getBoundingClientRect()
    return last.bottom <= box.bottom + 1 && last.right <= box.right + 1 && getComputedStyle(fitted).overflow === 'clip'
  }
  const drag = async (x: number, y: number, dx: number, dy: number) => {
    await window.smoke.pointer('mouseMove', Math.round(x), Math.round(y))
    await window.smoke.pointer('mouseDown', Math.round(x), Math.round(y))
    await window.smoke.pointer('mouseMove', Math.round(x + dx / 2), Math.round(y + dy / 2))
    await window.smoke.pointer('mouseMove', Math.round(x + dx), Math.round(y + dy))
    await window.smoke.pointer('mouseUp', Math.round(x + dx), Math.round(y + dy))
  }
  const select = async (id: string) => {
    const r = node(id).getBoundingClientRect()
    await drag(r.x + r.width / 2, r.y + r.height / 2, 0, 0)
    await until(() => node(id).classList.contains('selected'))
  }
  try {
    window.addEventListener('message', onArtifactMessage)
    root.render(<QueryClientProvider client={client}><Fixture /></QueryClientProvider>)
    await until(() => Boolean(node('report')?.querySelector('p')))
    await until(() => Boolean(linkIdentity), 'The external canvas link did not load')
    if (linkIdentity!.referrer || linkIdentity!.request.referer || /jaz|electron/i.test(linkIdentity!.userAgent)
      || linkIdentity!.request['user-agent'] !== linkIdentity!.userAgent) {
      throw new Error('The external canvas link received app identity or referrer metadata: ' + JSON.stringify(linkIdentity))
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
    await until(() => Boolean(node('cad')?.textContent?.includes('Could not open STL')))
    const status = node('cad').querySelector<HTMLElement>('.canvas-viewer-toolbar')!
    if (getComputedStyle(status).opacity === '0') {
      throw new Error('The CAD load error is hidden until the item is hovered or selected')
    }
    await select('report')
    const selection = window.getSelection()!
    const range = document.createRange()
    range.selectNodeContents(node('report').querySelector('p')!)
    selection.removeAllRanges()
    selection.addRange(range)
    for (const type of ['cut', 'copy']) {
      const event = new ClipboardEvent(type, { bubbles: true, cancelable: true, clipboardData: new DataTransfer() })
      node('report').dispatchEvent(event)
      if (event.defaultPrevented) {
        throw new Error(`${type} of selected document text was replaced with a canvas item`)
      }
    }
    selection.removeAllRanges()
    const copied = new DataTransfer()
    const copyItem = new ClipboardEvent('copy', { bubbles: true, cancelable: true, clipboardData: copied })
    node('report').dispatchEvent(copyItem)
    if (!copyItem.defaultPrevented || JSON.parse(copied.getData('application/x-jaz-canvas')).nodes[0]?.id !== 'report') {
      throw new Error('The canvas item could not be copied after clearing the text selection')
    }
    for (const id of ['report', 'table', 'sticky', 'text', 'artifact']) {
      const before = zoom()
      const r = node(id).getBoundingClientRect()
      await window.smoke.wheel(Math.round(r.x + r.width / 2), Math.round(r.y + r.height / 2), -30, true)
      await until(() => Math.abs(zoom() - before) > 0.001, `Pinch zoom was blocked over ${id}`)
      const next = node(id).getBoundingClientRect()
      await window.smoke.wheel(Math.round(next.x + next.width / 2), Math.round(next.y + next.height / 2), 30, true)
      await until(() => Math.abs(zoom() - before) < 0.001)
    }
    for (const id of ['report', 'table', 'sticky', 'text']) {
      await until(() => fits(id), `${id} content does not fit its card without scrolling`)
    }
    await until(() => Boolean(artifactLayout && artifactLayout.bottom <= artifactLayout.height + 1), 'The sandboxed card did not fit')
    await until(() => !element.querySelector('.canvas-busy'))
    await select('artifact')
    await until(() => !element.querySelector('.canvas-busy') && !node('artifact').querySelector('.canvas-shield'))
    const iframe = node('artifact').querySelector('iframe')!
    const frame = iframe.getBoundingClientRect()
    const button = artifactLayout!.button
    const buttonX = frame.x + (button.x + button.width / 2) * frame.width / iframe.clientWidth
    const buttonY = frame.y + (button.y + button.height / 2) * frame.height / iframe.clientHeight
    const hit = document.elementFromPoint(buttonX, buttonY)
    if (hit !== iframe) {
      throw new Error('The card button is covered by ' + hit?.className)
    }
    await drag(buttonX, buttonY, 0, 0)
    try {
      await until(() => Boolean(artifactLayout && artifactLayout.clicks > 0 && artifactLayout.bottom <= artifactLayout.height + 1))
    } catch {
      await window.smoke.capture('canvas-fit-interaction')
      throw new Error('The card failed to fit after interaction: ' + JSON.stringify(artifactLayout))
    }
    const beforeFramePinch = zoom()
    const frameAfter = iframe.getBoundingClientRect()
    await window.smoke.wheel(Math.round(frameAfter.x + frameAfter.width / 2), Math.round(frameAfter.y + frameAfter.height / 2), -30, true)
    await until(() => Math.abs(zoom() - beforeFramePinch) > 0.001, 'Pinch zoom was blocked inside the active sandbox')
    const frameNext = iframe.getBoundingClientRect()
    await window.smoke.wheel(Math.round(frameNext.x + frameNext.width / 2), Math.round(frameNext.y + frameNext.height / 2), 30, true)
    await until(() => Math.abs(zoom() - beforeFramePinch) < 0.001)
    element.querySelector<HTMLButtonElement>('[aria-label="Hand"]')!.click()
    await until(() => Boolean(element.querySelector('.tool-hand')))
    const report = node('report').getBoundingClientRect()
    const beforePan = viewport()
    await drag(report.x + report.width / 2, report.y + report.height / 2, 24, 0)
    await until(() => viewport() !== beforePan, 'The Hand tool could not pan over a document')
    await drag(report.x + report.width / 2 + 24, report.y + report.height / 2, -24, 0)
    element.querySelector<HTMLButtonElement>('[aria-label="Select"]')!.click()
    await until(() => Boolean(element.querySelector('.tool-select')))
    if (node('report').querySelector('.canvas-viewer-toolbar') || node('report').textContent?.includes('MD · report.md')) {
      throw new Error('The Markdown item still has a duplicate filename footer')
    }
    for (const zoomedOut of [false, true]) {
      if (zoomedOut) {
        for (let step = 0; step < 3; step++) {
          element.querySelector<HTMLButtonElement>('[aria-label="Zoom out"]')!.click()
          await new Promise((resolve) => setTimeout(resolve, 250))
        }
      }
      for (const id of ['report', 'page']) {
        await select(id)
        for (const corner of ['top left', 'top right', 'bottom right', 'bottom left']) {
          const r = node(id).getBoundingClientRect()
          const left = corner.includes('left')
          const top = corner.includes('top')
          const x = left ? r.left - 10 : r.right + 10
          const y = top ? r.top - 10 : r.bottom + 10
          const saved = view.nodes.find((item) => item.id === id)!
          const width = Number(saved.width)
          const height = Number(saved.height)
          const hit = document.elementFromPoint(x, y)
          if (!hit?.classList.contains('canvas-resize-handle')) {
            await window.smoke.capture('canvas-resize-miss')
            throw new Error(`${id} ${corner} at (${x}, ${y}) hit ${hit?.className}`)
          }
          await drag(x, y, left ? -15 : 15, top ? -15 : 15)
          await until(() => {
            const updated = view.nodes.find((item) => item.id === id)!
            return Number(updated.width) > width && Number(updated.height) > height
          }, `${id} ${corner} resize did not save (${width} × ${height}, zoomed out: ${zoomedOut})`)
        }
        const r = node(id).getBoundingClientRect()
        const width = Number(view.nodes.find((item) => item.id === id)!.width)
        await drag(r.right + 8, r.y + r.height / 2, 20, 0)
        await until(() => Number(view.nodes.find((item) => item.id === id)!.width) > width)
      }
    }
    view = { ...applyPatch(view, { nodes: [{ id: 'report', width: 220, height: 170 }, { id: 'artifact', width: 220, height: 160 }] }), version: String(Number(view.version) + 1) }
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
    await until(() => Number.parseFloat(node('report').style.height) === 170 && fits('report'), 'The resized document did not fit')
    await until(() => Boolean(artifactLayout && artifactLayout.height === 160 && artifactLayout.bottom <= 161), 'The resized sandbox did not fit')
    await select('page')
    const focus = node('page').querySelector<HTMLButtonElement>('.canvas-focus')!
    const r = focus.getBoundingClientRect()
    await drag(r.x + r.width / 2, r.y + r.height / 2, 0, 0)
    await until(() => node('page').querySelector('.canvas-focus')?.textContent === 'Done')
    await window.smoke.key('Escape')
    await until(() => node('page').querySelector('.canvas-focus')?.textContent === 'Focus')
    const label = node('report').querySelector<HTMLElement>('.canvas-item-label')!.getBoundingClientRect()
    const before = Number(view.nodes.find((item) => item.id === 'report')!.x)
    if (!document.elementFromPoint(label.x + label.width / 2, label.y + label.height / 2)?.closest('.canvas-item-label')) {
      throw new Error('Resize targets cover the document title')
    }
    await drag(label.x + label.width / 2, label.y + label.height / 2, 25, 0)
    await until(() => Number(view.nodes.find((item) => item.id === 'report')!.x) > before, 'The document could not be moved by its title')
    for (const id of ['sticky', 'text']) {
      view = { ...applyPatch(view, { nodes: [{ id, text: paragraphs + '\n\nAgent update' }] }), version: String(Number(view.version) + 1) }
      await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
      await until(() => Boolean(node(id).querySelector('.canvas-flash')))
      await until(() => fits(id), 'Updated note content did not fit')
      const frame = node(id).getBoundingClientRect()
      const flash = node(id).querySelector('.canvas-flash')!.getBoundingClientRect()
      if (Math.abs(flash.top - frame.top) > 1 || Math.abs(flash.height - frame.height) > 1) {
        throw new Error('An agent update highlight scrolled out of the note')
      }
    }
    await window.smoke.capture('canvas')
    root.render(<QueryClientProvider client={client}><Fixture inset={360} /></QueryClientProvider>)
    await until(() => element.querySelector<HTMLElement>('.canvas')!.style.getPropertyValue('--canvas-inset') === '360px')
    element.querySelector<HTMLButtonElement>('button[title="Fit everything"]')!.click()
    await until(() => Math.min(...view.nodes.map((item) => node(item.id).getBoundingClientRect().left)) >= element.getBoundingClientRect().left + 360, 'Fit placed items under the chat')
  } finally {
    window.removeEventListener('message', onArtifactMessage)
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
  }
}
