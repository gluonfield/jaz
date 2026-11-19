import type { SideBrowser } from '@/lib/sideBrowser'
import type { PreviewWebviewElement } from '@/components/session/previewWebview'

export async function exerciseGooglePasskeys(browser: SideBrowser): Promise<void> {
  const challenge = '/v3/signin/challenge/pk/presend'
  const google = 'https://accounts.google.com'
  const mac = /Mac/.test(navigator.platform)
  const opened = await window.smoke.openedURLs()
  await window.smoke.navigationProbe(true)
  try {
    for (const [url, restart] of [
      [google + challenge + '?cloud-fixture&TL=fixture#state', 'https://console.cloud.google.com/'],
      [google + challenge + '?TL=fixture', 'https://accounts.google.com/'],
      [google + '/v3/signin/challenge/pwd/presend', ''],
      ['https://accounts.google.com.example.test' + challenge, ''],
      ['http://accounts.google.com' + challenge, ''],
    ]) {
      await browser.call({ method: 'Jaz.open', params: { url } })
      const view = document.querySelector<PreviewWebviewElement>('webview')!
      const title = url.includes('cloud-fixture') ? 'Google Cloud Platform' : 'Address probe'
      const deadline = Date.now() + 5000
      while (view.getURL() !== url || view.getTitle() !== title || document.querySelector('.animate-spin')) {
        if (Date.now() >= deadline) throw new Error(`Passkey fixture did not finish navigating to ${url}`)
        await new Promise((resolve) => setTimeout(resolve, 20))
      }
      const handoff = [...document.querySelectorAll('button')].find((candidate) => candidate.textContent?.trim() === 'Open Google sign-in in browser')
      const openExternal = document.querySelector<HTMLButtonElement>('button[aria-label="Open in Browser"]')
      if (Boolean(handoff) !== Boolean(mac && restart) || Boolean(handoff) === Boolean(openExternal)) {
        throw new Error(`Incorrect browser handoff action at ${url}`)
      }
      if (JSON.stringify(await window.smoke.openedURLs()) !== JSON.stringify(opened)) throw new Error('Restarts must wait for a click')
      const button = handoff ?? openExternal!
      button.click()
      opened.push(restart || url)
      if (JSON.stringify(await window.smoke.openedURLs()) !== JSON.stringify(opened)) {
        throw new Error('The browser action opened the wrong URL')
      }
    }
  } finally {
    await window.smoke.navigationProbe(false)
  }
}
