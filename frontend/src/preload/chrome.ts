import { contextBridge } from 'electron'

declare const chrome: Record<string, unknown>

// Google sign-in rejects Chromium pages without Chrome's chrome.app, chrome.csi and chrome.loadTimes APIs as an embedded browser.
contextBridge.executeInMainWorld({
  func: () => {
    chrome.app = {
      isInstalled: false,
      InstallState: { DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed' },
      RunningState: { CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running' },
      getDetails: () => null,
      getIsInstalled: () => false,
      installState: (callback: (state: string) => void) => callback('not_installed'),
      runningState: () => 'cannot_run',
    }
    const navigation = () => performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined
    const epoch = (offset = 0) => offset && performance.timeOrigin + offset
    chrome.csi = () => {
      const entry = navigation()
      return {
        onloadT: Math.floor(epoch(entry?.loadEventEnd)),
        pageT: performance.now(),
        startE: Math.floor(performance.timeOrigin),
        tran: entry?.type === 'reload' ? 16 : entry?.type === 'back_forward' ? 6 : 15,
      }
    }
    chrome.loadTimes = () => {
      const entry = navigation()
      const protocol = entry?.nextHopProtocol ?? ''
      const multiplexed = protocol === 'h2' || protocol === 'h3'
      return {
        requestTime: performance.timeOrigin / 1000,
        startLoadTime: performance.timeOrigin / 1000,
        commitLoadTime: epoch(entry?.responseStart) / 1000,
        finishDocumentLoadTime: epoch(entry?.domContentLoadedEventEnd) / 1000,
        finishLoadTime: epoch(entry?.loadEventEnd) / 1000,
        firstPaintTime: epoch(performance.getEntriesByName('first-paint')[0]?.startTime) / 1000,
        firstPaintAfterLoadTime: 0,
        navigationType: entry?.type === 'reload' ? 'Reload' : entry?.type === 'back_forward' ? 'BackForward' : 'Other',
        wasFetchedViaSpdy: multiplexed,
        wasNpnNegotiated: multiplexed,
        npnNegotiatedProtocol: multiplexed ? protocol : 'unknown',
        wasAlternateProtocolAvailable: false,
        connectionInfo: protocol,
      }
    }
  },
})
