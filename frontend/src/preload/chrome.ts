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
    const transitions: Record<string, { navigationType: string; tran: number }> = {
      reload: { navigationType: 'Reload', tran: 16 },
      back_forward: { navigationType: 'BackForward', tran: 6 },
    }
    const navigation = () => {
      const entry = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming
      return { entry, ...transitions[entry.type] ?? { navigationType: 'Other', tran: 15 } }
    }
    // Chrome reports 0 for a milestone the page has not reached.
    const epoch = (offset = 0) => offset ? performance.timeOrigin + offset : 0
    chrome.csi = () => {
      const { entry, tran } = navigation()
      return { onloadT: Math.floor(epoch(entry.loadEventEnd)), pageT: performance.now(), startE: Math.floor(performance.timeOrigin), tran }
    }
    chrome.loadTimes = () => {
      const { entry, navigationType } = navigation()
      const protocol = entry.nextHopProtocol
      return {
        requestTime: performance.timeOrigin / 1000,
        startLoadTime: performance.timeOrigin / 1000,
        commitLoadTime: epoch(entry.responseStart) / 1000,
        finishDocumentLoadTime: epoch(entry.domContentLoadedEventEnd) / 1000,
        finishLoadTime: epoch(entry.loadEventEnd) / 1000,
        firstPaintTime: epoch(performance.getEntriesByName('first-paint')[0]?.startTime) / 1000,
        firstPaintAfterLoadTime: 0,
        navigationType,
        wasFetchedViaSpdy: protocol === 'h2' || protocol === 'h3',
        wasNpnNegotiated: protocol !== '',
        npnNegotiatedProtocol: protocol,
        wasAlternateProtocolAvailable: false,
        connectionInfo: protocol,
      }
    }
  },
})
