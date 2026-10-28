import { contextBridge } from 'electron'

declare const chrome: Record<string, unknown>

// Google sign-in rejects Chromium pages without Chrome's chrome.app API as an embedded browser.
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
  },
})
