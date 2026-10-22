import type { PreviewWebviewElement } from '@/components/session/previewWebview'
import type { SideBrowser } from '@/lib/sideBrowser'
import { getThemePref, setThemePref } from '@/lib/theme'

export async function exerciseBrowserAppearance(browser: SideBrowser, evaluate: (expression: string) => Promise<unknown>) {
  const theme = getThemePref()
  try {
    await browser.call({ method: 'Jaz.open', params: { url: location.origin + '/appearance' } })
    const page = document.querySelector<PreviewWebviewElement>('#root webview')!
    for (const appearance of ['dark', 'light'] as const) {
      setThemePref(appearance)
      for (const mode of ['', 'dark', 'scheme']) {
        await evaluate(`document.documentElement.className = ${JSON.stringify(mode)}`)
        await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
        await window.smoke.capture(`browser-appearance-${appearance}-${mode || 'transparent'}`)
        const image = new Image()
        image.src = (await page.capturePage()).toDataURL()
        await image.decode()
        const canvas = document.createElement('canvas')
        canvas.width = image.naturalWidth
        canvas.height = image.naturalHeight
        const context = canvas.getContext('2d')!
        context.drawImage(image, 0, 0)
        const pixel = [...context.getImageData(Math.floor(canvas.width / 2), Math.floor(canvas.height / 2), 1, 1).data]
        const expected = mode ? 18 : 255
        if (pixel[3] !== 255 || pixel.slice(0, 3).some(value => Math.abs(value - expected) > 2)) {
          throw new Error(`${mode || 'transparent'} page in ${appearance} Jaz painted ${pixel}, expected ${expected}`)
        }
      }
    }
  } finally {
    setThemePref(theme)
  }
}
