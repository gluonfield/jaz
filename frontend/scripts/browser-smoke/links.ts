import { apiBaseUrl, isLocalBackendUrl, setApiBaseUrl } from '@/lib/api/client'
import { previewDisplayUrl, resolvePreviewSource } from '@/lib/api/preview'
import type { SideBrowser } from '@/lib/sideBrowser'
import type { PreviewNavigationEvent, PreviewWebviewElement } from '@/components/session/previewWebview'

export async function exercisePreviewLinks(browser: SideBrowser, evaluate: (expression: string) => Promise<unknown>): Promise<void> {
  const backend = apiBaseUrl()
  const fetcher = window.fetch
  const original = await evaluate('location.href') as string
  const local = location.origin + '/target?preview=direct#potential-components'
  let proxyRequests = 0
  const unsubscribe = window.jaz!.onOpenPreviewURL(() => {})
  window.fetch = (input, init) => {
    if (String(input).endsWith('/v1/preview/proxies')) {
      proxyRequests += 1
      if (isLocalBackendUrl(apiBaseUrl())) {
        throw new Error('A local preview requested a proxy')
      }
    }
    return fetcher(input, init)
  }
  try {
    for (const host of ['localhost', '127.0.0.1', '[::1]', '0.0.0.0']) {
      setApiBaseUrl(`http://${host}:5299`)
      const url = `http://${host}:3000/app?query=one%20two#potential-components`
      if (await resolvePreviewSource(url) !== url) {
        throw new Error('A local preview URL was rewritten')
      }
    }
    await browser.call({ method: 'Jaz.open', params: { url: local } })
    if (await evaluate('location.href') !== local) {
      throw new Error('Local browser navigation changed the original URL')
    }
    const qrButton = document.querySelector<HTMLButtonElement>('[aria-label="Open on phone"]')!
    await until(() => !qrButton.disabled && document.querySelector<HTMLInputElement>('input[spellcheck="false"]')?.value === local)
    const rect = qrButton.getBoundingClientRect()
    const x = Math.round(rect.x + rect.width / 2)
    const y = Math.round(rect.y + rect.height / 2)
    const press = async () => {
      await window.smoke.pointer('mouseMove', x, y)
      await window.smoke.pointer('mouseDown', x, y)
      await window.smoke.pointer('mouseUp', x, y)
    }
    const qr = () => document.querySelector<HTMLImageElement>('img[alt="QR code for this page"]')
    await press()
    await until(() => {
      const panel = qr()?.closest<HTMLElement>('[data-escape-surface]')
      return Boolean(qr()?.naturalWidth && panel?.textContent === local && getComputedStyle(panel).opacity === '1')
    })
    await window.smoke.capture('browser-qr')
    await press()
    await until(() => !qr())
    if (proxyRequests !== 0) {
      throw new Error('Local previews must bypass proxy creation')
    }
    const appLink = location.origin + '/target?app-link'
    window.open(appLink, '_blank', 'noopener')
    await until(async () => (await window.smoke.tabURLs()).includes(appLink))
    if ((await window.smoke.openedURLs()).includes(appLink)) {
      throw new Error('An ordinary app link escaped to the external browser')
    }
    setApiBaseUrl(backend)
    const remoteSource = new URL(local)
    remoteSource.hostname = 'jaz-preview-fixture.localhost'
    if (await resolvePreviewSource(local) !== remoteSource.href || previewDisplayUrl(remoteSource.href) !== local) {
      throw new Error('Remote preview must load the proxy URL and display the original URL')
    }
    if (Number(proxyRequests) !== 1 || isLocalBackendUrl('https://server.example:5299')) {
      throw new Error('Remote and tunneled backends must retain preview proxying')
    }
    const publicURL = 'https://example.com/login?from=jaz#continue'
    if (await resolvePreviewSource(publicURL) !== publicURL || Number(proxyRequests) !== 1) {
      throw new Error('Public URLs must bypass proxy creation')
    }
    for (const [url, script] of [[location.origin + '/popup-login', false], [location.origin + '/popup-login-script', true]] as const) {
      const point = await evaluate(`(() => {
        const link = document.createElement('a')
        link.id = 'popup-fixture'
        link.href = ${JSON.stringify(url)}
        link.target = '_blank'
        link.rel = 'noopener noreferrer'
        link.textContent = 'Sign in'
        link.style.cssText = 'position:fixed;top:10px;left:10px;padding:16px;z-index:1000'
        if (${script}) {
          link.onclick = (event) => {
            event.preventDefault()
            window.open(link.href, '_blank', 'noopener')
          }
        }
        document.body.append(link)
        const rect = link.getBoundingClientRect()
        return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }
      })()`) as { x: number; y: number }
      await browser.call({ method: 'Input.dispatchMouseEvent', params: { type: 'mousePressed', ...point, button: 'left', clickCount: 1 } })
      await browser.call({ method: 'Input.dispatchMouseEvent', params: { type: 'mouseReleased', ...point, button: 'left', clickCount: 1 } })
      await until(async () => (await window.smoke.tabURLs()).includes(url))
      if ((await window.smoke.popupURLs()).includes(url)) {
        throw new Error('A new-tab link opened a popup window')
      }
      if ((await window.smoke.openedURLs()).includes(url)) {
        throw new Error('A browser popup escaped to the external browser')
      }
      if (await evaluate('location.href') !== local) {
        throw new Error('A new-tab link replaced the current preview')
      }
      await evaluate('document.getElementById("popup-fixture").remove()')
    }
    console.log('Local URLs, external toolbar, native new-tab links and window.open passed')
  } finally {
    unsubscribe()
    window.fetch = fetcher
    setApiBaseUrl(backend)
    await browser.call({ method: 'Jaz.open', params: { url: original } })
  }
}

async function until(check: () => boolean | Promise<boolean>): Promise<void> {
  const deadline = Date.now() + 3000
  while (!await check()) {
    if (Date.now() > deadline) {
      throw new Error('Preview URL or external browser opening did not complete')
    }
    await new Promise((resolve) => setTimeout(resolve, 20))
  }
}

export async function exerciseFrameNavigation(view: PreviewWebviewElement): Promise<void> {
  const local = view.getURL()
  const evaluate = (expression: string) => view.executeJavaScript(expression)
  const frameURL = new URL('/navigation-frame', location.origin)
  frameURL.hostname = 'localhost'
  const frameNavigated = new Promise<void>((resolve) => {
    view.addEventListener('did-navigate-in-page', function navigated(event) {
      const navigation = event as PreviewNavigationEvent
      if (navigation.isMainFrame === false && navigation.url === frameURL.href + '#check') {
        view.removeEventListener('did-navigate-in-page', navigated)
        resolve()
      }
    })
  })
  await evaluate(`new Promise(resolve => {
    const frame = document.createElement('iframe')
    frame.src = ${JSON.stringify(frameURL.href)}
    frame.onload = () => resolve()
    document.body.append(frame)
  })`)
  await new Promise((resolve) => setTimeout(resolve, 100))
  await evaluate('document.querySelector("iframe").contentWindow.postMessage("navigate", "*")')
  await frameNavigated
  await new Promise((resolve) => setTimeout(resolve, 250))
  if (!view.isConnected || await evaluate('location.href') !== local || view.closest('[data-browser-session]')!.querySelector<HTMLInputElement>('input[spellcheck="false"]')?.value !== local) {
    throw new Error('An embedded frame hash navigation replaced the main page or address')
  }
  await evaluate('document.querySelector("iframe").remove()')
  await evaluate('location.hash = "main-page"')
  await until(() => view.closest('[data-browser-session]')!.querySelector<HTMLInputElement>('input[spellcheck="false"]')?.value === local.split('#')[0] + '#main-page')
  await evaluate('history.back()')
  await until(() => view.closest('[data-browser-session]')!.querySelector<HTMLInputElement>('input[spellcheck="false"]')?.value === local)
}
