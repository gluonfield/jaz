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
      const destination = mac ? restart : ''
      const button = [...document.querySelectorAll('button')].find((candidate) => candidate.textContent === 'Open Google sign-in in browser')
      if (Boolean(button) !== Boolean(destination)) throw new Error(`Incorrect passkey handoff visibility at ${url}`)
      if (JSON.stringify(await window.smoke.openedURLs()) !== JSON.stringify(opened)) throw new Error('Restarts must wait for a click')
      if (!button) continue
      button.click()
      opened.push(destination)
      if (JSON.stringify(await window.smoke.openedURLs()) !== JSON.stringify(opened)) throw new Error('The handoff must restart at the fixed destination')
    }
  } finally {
    await window.smoke.navigationProbe(false)
  }
}
