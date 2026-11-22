import type { SideBrowser } from '@/lib/sideBrowser'
import type { PreviewWebviewElement } from '@/components/session/previewWebview'

export async function exerciseGooglePasskeys(browser: SideBrowser): Promise<void> {
  const challenge = '/v3/signin/challenge/pk/presend'
  const google = 'https://accounts.google.com'
  const other = 'https://passkey-site.example.test'
  const mac = /Mac/.test(navigator.platform)
  const opened = await window.smoke.openedURLs()
  const evaluate = async <T>(expression: string): Promise<T> => {
    const result = await browser.call({ method: 'Runtime.evaluate', params: { expression, returnByValue: true, awaitPromise: true } }) as { result: { value: T } }
    return result.result.value
  }
  const waitFor = async (check: () => boolean, description: string): Promise<void> => {
    const deadline = Date.now() + 5000
    while (!check()) {
      if (Date.now() >= deadline) {
        throw new Error(description)
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const navigate = async (url: string, title = 'Address probe'): Promise<void> => {
    await browser.call({ method: 'Jaz.open', params: { url } })
    const view = document.querySelector<PreviewWebviewElement>('webview')!
    await waitFor(
      () => view.getURL() === url && view.getTitle() === title && !document.querySelector('.animate-spin'),
      `Passkey fixture did not finish navigating to ${url}`,
    )
  }
  const notice = () => document.querySelector<HTMLElement>('[role="status"]')
  await window.smoke.navigationProbe(true)
  try {
    // 1. Google challenge URLs and lookalikes: notice visibility, click-only sanitized external restart.
    for (const [url, restart] of [
      [google + challenge + '?cloud-fixture&TL=fixture#state', 'https://console.cloud.google.com/'],
      [google + challenge + '?TL=fixture', 'https://accounts.google.com/'],
      [google + '/signin/v2/challenge/pk?TL=fixture', 'https://accounts.google.com/'],
      [google + '/v3/signin/challenge/pwd/presend', ''],
      ['https://accounts.google.com.example.test' + challenge, ''],
      ['http://accounts.google.com' + challenge, ''],
    ]) {
      const title = url.includes('cloud-fixture') ? 'Google Cloud Platform' : 'Address probe'
      await navigate(url, title)
      const handoff = [...document.querySelectorAll('button')].find((candidate) => candidate.textContent?.trim() === 'Open Google sign-in in browser')
      const openExternal = document.querySelector<HTMLButtonElement>('button[aria-label="Open in Browser"]')
      if (Boolean(handoff) !== Boolean(mac && restart) || !openExternal) {
        throw new Error(`Incorrect passkey handoff action at ${url}`)
      }
      if (JSON.stringify(await window.smoke.openedURLs()) !== JSON.stringify(opened)) {
        throw new Error('Restarts must wait for a click')
      }
      const button = handoff ?? openExternal
      button.click()
      opened.push(restart || url)
      if (JSON.stringify(await window.smoke.openedURLs()) !== JSON.stringify(opened)) {
        throw new Error('The browser action opened the wrong URL')
      }
    }

    if (!mac) {
      return
    }

    // 2. Arbitrary HTTPS site: truthful WebAuthn capabilities and silent background mediation.
    await navigate(other + '/login')
    if (notice()) {
      throw new Error('Passkey notice appeared before any WebAuthn request on an ordinary page')
    }
    const probe = await evaluate<{
      cma: boolean
      uvpaa: boolean
      caps: Record<string, boolean>
      conditionalErr: string
      immediateErr: string
      abortedErr: string
      nonPublicKey: string
      usbKeyErr: string
      hintKeyErr: string
      crossPlatformCreateErr: string
    }>(`(async () => {
      const ch = new Uint8Array(32)
      const fail = async (opts) => {
        try {
          const result = await navigator.credentials.get(opts)
          return result === null ? 'null' : 'resolved'
        } catch (err) {
          return err.name
        }
      }
      const abort = new AbortController()
      abort.abort()
      const timed = (fn) => {
        const ctrl = new AbortController()
        const timer = setTimeout(() => ctrl.abort(), 120)
        return fn(ctrl.signal).then(() => 'resolved', (err) => err.name).finally(() => clearTimeout(timer))
      }
      return {
        cma: await PublicKeyCredential.isConditionalMediationAvailable(),
        uvpaa: await PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable(),
        caps: await PublicKeyCredential.getClientCapabilities(),
        conditionalErr: await fail({ mediation: 'conditional', publicKey: { challenge: ch, rpId: location.hostname } }),
        immediateErr: await fail({ mediation: 'immediate', publicKey: { challenge: ch, rpId: location.hostname } }),
        abortedErr: await fail({ signal: abort.signal, publicKey: { challenge: ch, rpId: location.hostname } }),
        nonPublicKey: await fail({}),
        usbKeyErr: await timed((signal) => navigator.credentials.get({
          signal,
          publicKey: { challenge: ch, rpId: location.hostname, allowCredentials: [{ type: 'public-key', id: new Uint8Array(16), transports: ['usb'] }] },
        })),
        hintKeyErr: await timed((signal) => navigator.credentials.get({
          signal,
          publicKey: { challenge: ch, rpId: location.hostname, hints: ['security-key'] },
        })),
        crossPlatformCreateErr: await timed((signal) => navigator.credentials.create({
          signal,
          publicKey: {
            challenge: ch,
            rp: { name: 'Fixture', id: location.hostname },
            user: { id: new Uint8Array(16), name: 'user', displayName: 'User' },
            pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
            authenticatorSelection: { authenticatorAttachment: 'cross-platform' },
          },
        })),
      }
    })()`)
    if (
      probe.cma || probe.uvpaa
      || probe.caps.passkeyPlatformAuthenticator || probe.caps.userVerifyingPlatformAuthenticator
      || probe.caps.hybridTransport || probe.caps.conditionalGet || probe.caps.conditionalCreate || probe.caps.immediateGet
    ) {
      throw new Error(`WebAuthn capabilities still over-report unsupported passkeys: ${JSON.stringify(probe)}`)
    }
    if (
      probe.conditionalErr !== 'NotSupportedError'
      || probe.immediateErr !== 'NotAllowedError'
      || probe.abortedErr !== 'AbortError'
      || probe.nonPublicKey !== 'NotSupportedError'
      || probe.usbKeyErr !== 'AbortError'
      || probe.hintKeyErr !== 'AbortError'
      || probe.crossPlatformCreateErr !== 'AbortError'
    ) {
      throw new Error(`Unexpected passkey filter behavior: ${JSON.stringify(probe)}`)
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
    if (notice()) {
      throw new Error('Silent, non-publicKey, and explicit security-key requests must not show the passkey notice')
    }

    // 3. Interactive get() and create() reject immediately (never hang) and surface the notice with actions.
    const interactive = await evaluate<{ getErr: string; getMs: number; createErr: string; createMs: number }>(`(async () => {
      const ch = new Uint8Array(32)
      const measure = async (fn) => {
        const t0 = performance.now()
        try {
          await fn()
          return { err: 'resolved', ms: performance.now() - t0 }
        } catch (e) {
          return { err: e.name, ms: performance.now() - t0 }
        }
      }
      const g = await measure(() => navigator.credentials.get({
        publicKey: {
          challenge: ch,
          rpId: location.hostname,
          allowCredentials: [{ type: 'public-key', id: new Uint8Array(16), transports: ['internal', 'hybrid'] }],
          userVerification: 'preferred',
          timeout: 60000,
        },
      }))
      const c = await measure(() => navigator.credentials.create({
        publicKey: {
          challenge: ch,
          rp: { name: 'Fixture', id: location.hostname },
          user: { id: new Uint8Array(16), name: 'user', displayName: 'User' },
          pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
          authenticatorSelection: { authenticatorAttachment: 'platform' },
          timeout: 60000,
        },
      }))
      return { getErr: g.err, getMs: g.ms, createErr: c.err, createMs: c.ms }
    })()`)
    if (interactive.getErr !== 'NotAllowedError' || interactive.createErr !== 'NotAllowedError' || interactive.getMs > 500 || interactive.createMs > 500) {
      throw new Error(`Interactive passkey requests did not fail fast cleanly: ${JSON.stringify(interactive)}`)
    }
    await waitFor(() => Boolean(notice()), 'Interactive passkey request did not show the passkey notice')
    const openInBrowser = [...notice()!.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Open in browser')
    const importCookies = [...notice()!.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Import cookies')
    if (!openInBrowser || !importCookies) {
      throw new Error('Passkey notice is missing Open in browser or Import cookies actions')
    }
    openInBrowser.click()
    opened.push(other + '/login')
    if (JSON.stringify(await window.smoke.openedURLs()) !== JSON.stringify(opened)) {
      throw new Error('Open in browser from passkey notice opened the wrong URL')
    }
    importCookies.click()
    await waitFor(() => Boolean(document.querySelector('[role="dialog"]')), 'Import cookies did not open the profile import dialog')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await waitFor(() => !document.querySelector('[role="dialog"]'), 'Profile import dialog did not close')

    // 4. Dismiss hides notice, retrying passkey brings it back, navigating away clears it.
    document.querySelector<HTMLButtonElement>('button[aria-label="Dismiss passkey notice"]')!.click()
    await waitFor(() => !notice(), 'Dismissing passkey notice did not hide it')
    await evaluate(`navigator.credentials.get({ publicKey: { challenge: new Uint8Array(32), rpId: location.hostname, timeout: 60000 } }).catch(() => {})`)
    await waitFor(() => Boolean(notice()), 'Retrying a passkey request after dismissal did not re-show the notice')
    await navigate(other + '/signed-in')
    await waitFor(() => !notice(), 'Navigating away did not clear the passkey notice')

    // 5. Popup windows also report truthful capabilities, fail fast, and notify the opener tab.
    const popupResult = await evaluate<{ cma: boolean; passkeyPlatformAuthenticator: boolean; hybridTransport: boolean; err: string }>(`new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('Popup passkey check timed out')), 4000)
      window.addEventListener('message', (event) => {
        clearTimeout(timer)
        resolve(event.data)
      }, { once: true })
      window.open(${JSON.stringify(google + '/v3/signin/challenge/sk?popup-passkey')}, '_blank', 'width=500,height=600')
    })`)
    if (popupResult.cma || popupResult.passkeyPlatformAuthenticator || popupResult.hybridTransport || popupResult.err !== 'NotAllowedError') {
      throw new Error(`Popup passkey fallback failed: ${JSON.stringify(popupResult)}`)
    }
    await waitFor(() => Boolean(notice()), 'Popup passkey block did not notify the opener browser tab')
  } finally {
    await window.smoke.navigationProbe(false)
  }
}
