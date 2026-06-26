export {}

import type { BrowserNavigationDirection } from '../shared/browserNavigation'
import type { UpdateStatus } from '../shared/update'

declare global {
  interface Window {
    jaz?: {
      apiBaseUrl: string
      windowKind: 'main' | 'board' | 'launcher'
      setNativeTheme: (source: 'light' | 'dark' | 'system') => void
      startLocalBackend: () => Promise<{ ok: boolean; url?: string; key?: string; error?: string }>
      getDeviceIdentity: () => Promise<{ device_id: string; public_key: string }>
      getDeviceMetadata: () => Promise<{
        name: string
        platform: string
        device_family: string
        model_identifier: string
        app_version: string
      }>
      getUpdateStatus: () => Promise<UpdateStatus>
      installUpdate: () => Promise<{ ok: boolean; error?: string }>
      onUpdateStatus: (handler: (status: UpdateStatus) => void) => () => void
      openBoardWindow: (boardId: string) => void
      openExternalURL: (url: string) => void
      captureScreenRect: (rect: {
        x: number
        y: number
        width: number
        height: number
      }) => Promise<{ ok: boolean; data?: string; denied?: boolean }>
      hideLauncher: () => void
      onLauncherShown: (handler: () => void) => () => void
      openInMain: (path: string) => void
      onOpenRoute: (handler: (path: string) => void) => () => void
      onOpenPreviewURL: (handler: (url: string) => void) => () => void
      onBrowserNavigation: (handler: (direction: BrowserNavigationDirection) => void) => () => void
    }
  }
}
