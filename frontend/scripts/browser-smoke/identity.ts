export async function exerciseBrowserIdentity(evaluate: (expression: string) => Promise<unknown>): Promise<void> {
  const appRequest = await fetch('/browser-identity').then(response => response.json()) as { request: Record<string, string> }
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
      workerUserAgent, webdriver: navigator.webdriver, referrer: document.referrer,
      pageGlobals: Object.getOwnPropertyNames(window).filter(name => /jaz/i.test(name)),
      hints: await navigator.userAgentData.getHighEntropyValues(['architecture', 'bitness', 'model', 'platformVersion', 'fullVersionList']),
      chromeApp: chrome.app.runningState() }
  })()`) as {
    firstNavigation: Record<string, string>
    request: Record<string, string>
    chromeVersion: string
    appUserAgent: string
    userAgent: string
    workerUserAgent: string
    webdriver: boolean
    referrer: string
    pageGlobals: string[]
    chromeApp: string
    language: string
    hints: { brands: { brand: string; version: string }[]; platform: string; mobile: boolean; fullVersionList: { brand: string; version: string }[] }
  }
  const { firstNavigation, request, chromeVersion, userAgent, workerUserAgent, hints } = observed
  if (/\b(?:Jaz|Electron)\//.test(userAgent) || !userAgent.includes(`Chrome/${chromeVersion}`)) {
    throw new Error('The side browser must identify its actual Chromium engine without Electron/Jaz product tokens')
  }
  if (firstNavigation['user-agent'] !== userAgent || request['user-agent'] !== userAgent || workerUserAgent !== userAgent) {
    throw new Error('First navigation, fetch, page and worker browser identities disagree')
  }
  if (observed.appUserAgent !== userAgent || appRequest.request['user-agent'] !== userAgent) {
    throw new Error('App resource requests must use the same native Chromium identity as the side browser')
  }
  if (observed.pageGlobals.length || observed.referrer || firstNavigation.referer
    || /jaz|electron/i.test(JSON.stringify(hints))
    || [firstNavigation, request, appRequest.request].some(headers => Object.keys(headers).some(name => /jaz/i.test(name)))) {
    throw new Error('The visited page received Jaz/Electron metadata or an app referrer')
  }
  const chrome = hints.brands.find(({ brand }) => brand === 'Chromium')
  if (chrome?.version !== chromeVersion.split('.')[0] || hints.fullVersionList.find(({ brand }) => brand === 'Chromium')?.version !== chromeVersion
    || request['sec-ch-ua-platform'] !== JSON.stringify(hints.platform)
    || request['sec-ch-ua-mobile'] !== (hints.mobile ? '?1' : '?0')
    || request['sec-ch-ua'] !== hints.brands.map(({ brand, version }) => `${JSON.stringify(brand)};v=${JSON.stringify(version)}`).join(', ')) {
    throw new Error('Native Chromium client hints disagree with browser headers')
  }
  if (observed.chromeApp !== 'cannot_run') {
    throw new Error('Side-browser pages must expose Chrome\'s chrome.app API for Google sign-in')
  }
  if (request['accept-language'].split(',')[0].split(';')[0] !== observed.language || observed.webdriver) {
    throw new Error('The normal browser language or driver state changed')
  }
  console.warn(JSON.stringify({ browserIdentity: observed, appRequest: appRequest.request }))
}
