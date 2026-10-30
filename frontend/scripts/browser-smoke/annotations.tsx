import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { PreviewPanel } from '@/components/session/PreviewPanel'
import type { PreviewWebviewElement } from '@/components/session/previewWebview'
import { useComposerContexts } from '@/components/session/useComposerContexts'
import { ContextChip } from '@/components/session/ContextChip'

export async function exerciseAnnotations(): Promise<void> {
  const element = document.createElement('div')
  element.style.cssText = 'position:absolute;inset:0;background:white;display:flex;justify-content:flex-end'
  document.body.append(element)
  const root = createRoot(element)
  function Fixture() {
    const [target, setTarget] = useState({ displayUrl: location.origin + '/target', sourceUrl: location.origin + '/target' })
    const [revision, setRevision] = useState(0)
    const composer = useComposerContexts({ storageKey: 'annotation-fixture', storage: 'session' })
    return <>
      <div className="absolute left-4 top-4 max-w-48">
        <button type="button" onClick={() => setRevision(revision + 1)} data-refresh>Update renderer {revision}</button>
        {composer.contexts.map((context, index) => <ContextChip key={context.id} index={index} context={context} />)}
      </div>
      <PreviewPanel target={target} onTargetChange={(next) => setTarget(next)} onAddBrowserAnnotation={composer.addBrowserAnnotation} onClose={() => {}} />
    </>
  }
  const until = async (check: () => boolean | Promise<boolean>) => {
    const end = Date.now() + 5000
    while (!await check()) {
      if (Date.now() > end) {
        throw new Error('Annotation browser check timed out')
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const click = async (button: HTMLElement) => {
    const rect = button.getBoundingClientRect()
    const x = Math.round(rect.x + rect.width / 2)
    const y = Math.round(rect.y + rect.height / 2)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const annotate = () => element.querySelector<HTMLButtonElement>('[aria-label="Annotate preview"]')!
  try {
    root.render(<Fixture />)
    await until(() => Boolean(annotate() && !annotate().disabled))
    const webview = element.querySelector('webview') as PreviewWebviewElement
    const command = (method: string, params: Record<string, unknown>) => window.jaz!.browserCommand({ webContentsId: webview.getWebContentsId(), method, params })
    const evaluate = async (expression: string) => {
      const response = await command('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }) as { result: { value: unknown } }
      return response.result.value
    }
    await click(element.querySelector<HTMLElement>('[data-refresh]')!)
    await new Promise((resolve) => setTimeout(resolve, 80))
    if (annotate().disabled) {
      throw new Error('Updating the renderer disabled annotation on an already loaded page')
    }
    await click(annotate())
    await until(async () => await evaluate('typeof window.__browserAnnotationCancel') === 'function')
    if (await evaluate(`Object.getOwnPropertyNames(window).some(name => /jaz/i.test(name)) || Boolean(document.querySelector('[data-jaz-annotation-ui]'))`)) {
      throw new Error('Annotation exposed Jaz branding to the visited page')
    }
    const point = await evaluate(`(() => {
      const r = document.querySelector('h1').getBoundingClientRect()
      return { x: r.x + 20, y: r.y + 20 }
    })()`) as { x: number; y: number }
    await command('Input.dispatchMouseEvent', { type: 'mousePressed', ...point, button: 'left', clickCount: 1 })
    await command('Input.dispatchMouseEvent', { type: 'mouseReleased', ...point, button: 'left', clickCount: 1 })
    await command('Input.insertText', { text: 'Make this heading clearer' })
    await evaluate('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))')
    await new Promise((resolve) => setTimeout(resolve, 100))
    const editorFits = await evaluate(`(() => {
      const form = document.querySelector('[data-browser-annotation-ui]').shadowRoot.querySelector('form')
      const rect = form.getBoundingClientRect()
      return rect.width <= 350 && rect.height <= 180 && [...form.querySelectorAll('button')].every(button => {
        const buttonRect = button.getBoundingClientRect()
        return buttonRect.left >= rect.left && buttonRect.right <= rect.right
      })
    })()`)
    if (!editorFits) {
      throw new Error('Page styles distorted the annotation editor')
    }
    await window.smoke.capture('browser-annotation')
    const submitPoint = await evaluate(`(() => {
      const r = document.querySelector('[data-browser-annotation-ui]').shadowRoot.querySelector('[type="submit"]').getBoundingClientRect()
      return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
    })()`) as { x: number; y: number }
    await command('Input.dispatchMouseEvent', { type: 'mousePressed', ...submitPoint, button: 'left', clickCount: 1 })
    await command('Input.dispatchMouseEvent', { type: 'mouseReleased', ...submitPoint, button: 'left', clickCount: 1 })
    await until(() => element.textContent!.includes('Make this heading clearer'))
    const contexts = JSON.parse(sessionStorage.getItem('annotation-fixture.contexts')!)
    if (contexts.length !== 1 || contexts[0].browser_annotation.target !== 'Browser control') {
      throw new Error('Annotation did not reach the composer with its selected page element')
    }
    await until(async () => await evaluate('typeof window.__browserAnnotationCancel') === 'undefined')
    await click(annotate())
    await until(() => Boolean(element.querySelector('[aria-label="Stop annotation"]')))
    await click(element.querySelector<HTMLElement>('[aria-label="Stop annotation"]')!)
    await until(() => Boolean(annotate() && !annotate().disabled))
    if (JSON.parse(sessionStorage.getItem('annotation-fixture.contexts')!).length !== 1) {
      throw new Error('Cancelling annotation added an empty composer context')
    }
  } finally {
    root.unmount()
    element.remove()
    sessionStorage.removeItem('annotation-fixture.contexts')
  }
}
