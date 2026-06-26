import type { BrowserNavigationDirection } from '../../../shared/browserNavigation'
import type { UpdateStatus } from '../../../shared/update'

export const DEFAULT_API_BASE_URL = 'http://localhost:5299'

export type ClientRuntimeKind = 'electron' | 'web'
export type ClientPlatform = 'desktop' | 'browser'
export type ClientWindowKind = 'main' | 'board' | 'launcher'

export interface ClientRuntime {
  kind: ClientRuntimeKind
  platform: ClientPlatform
  deviceKind: ClientPlatform
  windowKind: ClientWindowKind
  capabilities: {
    localBackend: boolean
    updates: boolean
    previewWebview: boolean
  }
  defaultApiBaseUrl: () => string
  setNativeTheme?: (source: 'light' | 'dark' | 'system') => void
  startLocalBackend?: () => Promise<{ ok: boolean; url?: string; key?: string; error?: string }>
  getDeviceIdentity?: () => Promise<{ device_id: string; public_key: string }>
  getDeviceMetadata?: () => Promise<{
    name: string
    platform: string
    device_family: string
    model_identifier: string
    app_version: string
  }>
  getUpdateStatus?: () => Promise<UpdateStatus>
  installUpdate?: () => Promise<{ ok: boolean; error?: string }>
  onUpdateStatus?: (handler: (status: UpdateStatus) => void) => () => void
  openBoardWindow?: (boardId: string) => void
  openExternalURL?: (url: string) => void
  captureScreenRect?: (rect: { x: number; y: number; width: number; height: number }) => Promise<{ ok: boolean; data?: string }>
  hideLauncher?: () => void
  onLauncherShown?: (handler: () => void) => () => void
  openInMain?: (path: string) => void
  onOpenRoute?: (handler: (path: string) => void) => () => void
  onOpenPreviewURL?: (handler: (url: string) => void) => () => void
  onBrowserNavigation?: (handler: (direction: BrowserNavigationDirection) => void) => () => void
}

function webDefaultApiBaseUrl(): string {
  const configured = import.meta.env.VITE_JAZ_API_URL?.trim()
  // "origin" targets the origin the app is served from, so one build works
  // behind a reverse proxy at any domain (app and API share an origin).
  if (configured === 'origin') return window.location.origin
  if (configured) return configured
  return DEFAULT_API_BASE_URL
}

function createRuntime(): ClientRuntime {
  const electron = window.jaz
  if (electron) {
    return {
      kind: 'electron',
      platform: 'desktop',
      deviceKind: 'desktop',
      windowKind: electron.windowKind,
      capabilities: {
        localBackend: true,
        updates: true,
        previewWebview: true,
      },
      defaultApiBaseUrl: () => electron.apiBaseUrl,
      setNativeTheme: electron.setNativeTheme,
      startLocalBackend: electron.startLocalBackend,
      getDeviceIdentity: electron.getDeviceIdentity,
      getDeviceMetadata: electron.getDeviceMetadata,
      getUpdateStatus: electron.getUpdateStatus,
      installUpdate: electron.installUpdate,
      onUpdateStatus: electron.onUpdateStatus,
      openBoardWindow: electron.openBoardWindow,
      openExternalURL: electron.openExternalURL,
      captureScreenRect: electron.captureScreenRect,
      hideLauncher: electron.hideLauncher,
      onLauncherShown: electron.onLauncherShown,
      openInMain: electron.openInMain,
      onOpenRoute: electron.onOpenRoute,
      onOpenPreviewURL: electron.onOpenPreviewURL,
      onBrowserNavigation: electron.onBrowserNavigation,
    }
  }
  return {
    kind: 'web',
    platform: 'browser',
    deviceKind: 'browser',
    windowKind: 'main',
    capabilities: {
      localBackend: false,
      updates: false,
      previewWebview: false,
    },
    defaultApiBaseUrl: webDefaultApiBaseUrl,
  }
}

export const clientRuntime = createRuntime()
