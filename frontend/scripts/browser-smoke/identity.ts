export async function exerciseBrowserIdentity(evaluate: (expression: string) => Promise<unknown>): Promise<void> {
  const observed = await evaluate(`(async () => {
    const network = await fetch('/browser-identity').then(response => response.json())
    const workerUserAgent = await new Promise((resolve, reject) => {
      const url = URL.createObjectURL(new Blob(['postMessage(navigator.userAgent)'], { type: 'text/javascript' }))
      const worker = new Worker(url)
      worker.onerror = reject
      worker.onmessage = event => {
        resolve(event.data)
        worker.terminate()
        URL.revokeObjectURL(url)
      }
    })
    return { ...network, userAgent: navigator.userAgent, language: navigator.language,
      workerUserAgent, webdriver: navigator.webdriver, hints: navigator.userAgentData.toJSON() }
  })()`) as {
    firstNavigation: Record<string, string>
    request: Record<string, string>
    chromeVersion: string
    shellUserAgent: string
    userAgent: string
    workerUserAgent: string
    webdriver: boolean
    language: string
    hints: { brands: { brand: string; version: string }[]; platform: string; mobile: boolean }
  }
  const { firstNavigation, request, chromeVersion, userAgent, workerUserAgent, hints } = observed
  if (/\b(?:Jaz|Electron)\//.test(userAgent) || !userAgent.includes(`Chrome/${chromeVersion}`)) {
    throw new Error('The side browser must identify its actual Chromium engine without Electron/Jaz product tokens')
  }
  if (firstNavigation['user-agent'] !== userAgent || request['user-agent'] !== userAgent || workerUserAgent !== userAgent) {
    throw new Error('First navigation, fetch, page and worker browser identities disagree')
  }
  if (!observed.shellUserAgent.includes('Jaz/') || !observed.shellUserAgent.includes('Electron/')) {
    throw new Error('Side-browser identity configuration changed the app session')
  }
  const chrome = hints.brands.find(({ brand }) => brand === 'Chromium')
  if (chrome?.version !== chromeVersion.split('.')[0] || request['sec-ch-ua-platform'] !== JSON.stringify(hints.platform)
    || request['sec-ch-ua-mobile'] !== (hints.mobile ? '?1' : '?0')
    || request['sec-ch-ua'] !== hints.brands.map(({ brand, version }) => `${JSON.stringify(brand)};v=${JSON.stringify(version)}`).join(', ')) {
    throw new Error('Native Chromium client hints disagree with browser headers')
  }
  if (request['accept-language'].split(',')[0].split(';')[0] !== observed.language || observed.webdriver) {
    throw new Error('The normal browser language or driver state changed')
  }
}
