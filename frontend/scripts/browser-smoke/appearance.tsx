import { createRoot } from 'react-dom/client'
import { PreviewPanel } from '@/components/session/PreviewPanel'
import type { PreviewWebviewElement } from '@/components/session/previewWebview'
import { clientRuntime } from '@/lib/clientRuntime'
import { getThemePref, setThemePref } from '@/lib/theme'

export async function exerciseBrowserAppearance() {
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;inset:0;z-index:9999;display:flex;background:var(--color-bg)'
  document.body.append(host)
  const root = createRoot(host)
  const theme = getThemePref()
  const native = clientRuntime.capabilities.previewWebview
  try {
    for (const webview of [true, false]) {
      clientRuntime.capabilities.previewWebview = webview
      root.render(<PreviewPanel key={String(webview)} embedded target={{ displayUrl: location.origin + '/appearance', sourceUrl: location.origin + '/appearance' }} onTargetChange={() => {}} />)
      let page: PreviewWebviewElement | HTMLIFrameElement | null = null
      const deadline = performance.now() + 10000
      const evaluate = async (expression: string) => {
        page = host.querySelector<PreviewWebviewElement | HTMLIFrameElement>(webview ? 'webview' : 'iframe')
        if (!page) {
          return false
        }
        if (page instanceof HTMLIFrameElement) {
          return page.contentWindow?.eval(expression)
        }
        try {
          return await page.executeJavaScript(expression)
        } catch {
          return false
        }
      }
      while (!await evaluate('document.querySelector("h1")?.textContent === "Readable page"')) {
        if (performance.now() > deadline) {
          throw new Error('Browser appearance page did not load')
        }
        await new Promise((resolve) => setTimeout(resolve, 25))
      }
      for (const appearance of ['dark', 'light'] as const) {
        setThemePref(appearance)
        for (const darkPage of [false, true]) {
          await evaluate(`document.documentElement.classList.toggle('dark', ${darkPage})`)
          await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
          const image = new Image()
          image.src = await window.smoke.capture(`browser-appearance-${webview ? 'native' : 'iframe'}-${appearance}-${darkPage ? 'dark' : 'transparent'}`)
          await image.decode()
          const canvas = document.createElement('canvas')
          canvas.width = image.naturalWidth
          canvas.height = image.naturalHeight
          const context = canvas.getContext('2d')!
          context.drawImage(image, 0, 0)
          const bounds = page!.getBoundingClientRect()
          const scale = image.naturalWidth / innerWidth
          const pixel = [...context.getImageData(Math.floor((bounds.x + bounds.width / 2) * scale), Math.floor((bounds.y + bounds.height / 2) * scale), 1, 1).data]
          const expected = darkPage ? 18 : 255
          if (pixel.slice(0, 3).some(value => Math.abs(value - expected) > 2)) {
            throw new Error(`${webview ? 'Webview' : 'Iframe'} ${darkPage ? 'dark' : 'transparent'} page in ${appearance} Jaz painted ${pixel}, expected ${expected}`)
          }
        }
      }
    }
  } finally {
    root.unmount()
    host.remove()
    clientRuntime.capabilities.previewWebview = native
    setThemePref(theme)
  }
}
