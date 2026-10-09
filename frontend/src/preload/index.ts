import { contextBridge, ipcRenderer, webUtils } from 'electron'
import { COMPUTER_CHANNEL, PERMISSION_GUIDE_CHANNEL, type ComputerAPI, type PermissionGuideAPI } from '@shared/computerControl'
import { browserPasswords } from '@preload/browserPasswords'
import { browserDownloads } from '@preload/browserDownloads'
import { installBrowserPasswordCapture } from '@preload/browser'
import { BROWSER_PRELOAD_ARGUMENT } from '@shared/preview'
import { BROWSER_COMMAND_CHANNEL, type BrowserCommandRequest } from '@shared/browserControl'
import { BROWSER_PROFILE_CHANNELS, type BrowserProfileAPI } from '@shared/browserProfile'
import {
  BROWSER_NAVIGATION_CHANNEL,
  type BrowserNavigationDirection,
} from '../shared/browserNavigation'
import { PREVIEW_FIND_SHORTCUT_CHANNEL } from '../shared/previewFind'
import type { NotificationsAPI, ThreadNotificationConfig } from '../shared/notifications'
import type { UpdateStatus } from '../shared/update'
import type { DictationAPI, DictationEvent } from '../shared/dictation'
import type { VoiceCommand, VoiceOverlayAPI, VoiceOverlayState } from '@shared/voice'

const apiBaseUrl = process.env['JAZ_API_URL'] ?? 'http://127.0.0.1:5299'

// Secondary windows are spawned with a flag so the renderer can drop
// the app chrome (sidebar, titlebar) and render that surface full-bleed.
const windowKind = (['board', 'voice', 'launcher', 'permission'] as const).find((kind) => process.argv.includes(`--jaz-${kind}-window`)) ?? 'main'
let previewURLTargetSubscriptions = 0

if (process.argv.includes(BROWSER_PRELOAD_ARGUMENT)) {
  installBrowserPasswordCapture()
} else {
  contextBridge.exposeInMainWorld('jaz', {
    browserPasswords,
    browserDownloads,
    computer: {
      status: () => ipcRenderer.invoke(COMPUTER_CHANNEL + 'status'),
      allow: (permission) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'allow', permission),
      begin: (id, session) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'begin', id, session),
      call: (id, action) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'call', id, action),
      end: (id) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'end', id),
      cancel: (id) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'cancel', id),
    } satisfies ComputerAPI,
    permissionGuide: {
      drag: () => ipcRenderer.send(PERMISSION_GUIDE_CHANNEL + 'drag'),
      close: () => ipcRenderer.send(PERMISSION_GUIDE_CHANNEL + 'close'),
    } satisfies PermissionGuideAPI,
    voiceOverlay: {
      drag: (point) => ipcRenderer.send('jaz:voice:drag', point),
      publish: (state) => ipcRenderer.send('jaz:voice:publish', state),
      subscribe: (handler) => {
        const listener = (_event: unknown, state: VoiceOverlayState | null) => handler(state)
        ipcRenderer.on('jaz:voice:state', listener)
        ipcRenderer.send('jaz:voice:ready')
        return () => ipcRenderer.removeListener('jaz:voice:state', listener)
      },
      command: (command) => ipcRenderer.send('jaz:voice:command', command),
      onCommand: (handler) => {
        const listener = (_event: unknown, command: VoiceCommand) => handler(command)
        ipcRenderer.on('jaz:voice:command', listener)
        return () => ipcRenderer.removeListener('jaz:voice:command', listener)
      },
    } satisfies VoiceOverlayAPI,
    dictation: {
      availability: () => ipcRenderer.invoke('jaz:dictation:availability'),
      start: (id) => ipcRenderer.invoke('jaz:dictation:start', id),
      stop: (id) => ipcRenderer.invoke('jaz:dictation:stop', id),
      cancel: (id) => ipcRenderer.invoke('jaz:dictation:cancel', id),
      onEvent: (handler) => {
        const listener = (_event: unknown, id: string, event: DictationEvent) => handler(id, event)
        ipcRenderer.on('jaz:dictation:event', listener)
        return () => ipcRenderer.removeListener('jaz:dictation:event', listener)
      },
    } satisfies DictationAPI,
    browserProfiles: {
      list: () => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.list),
      import: (profileId, selection) => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.import, profileId, selection),
    } satisfies BrowserProfileAPI,
    browserCommand: (request: BrowserCommandRequest): Promise<unknown> =>
      ipcRenderer.invoke(BROWSER_COMMAND_CHANNEL, request),
    apiBaseUrl,
    windowKind,
    pathForFile: (file: File) => webUtils.getPathForFile(file),
    setNativeTheme: (source: 'light' | 'dark' | 'system') =>
      ipcRenderer.send('jaz:set-native-theme', source),
    startLocalBackend: (): Promise<{ ok: boolean; url?: string; key?: string; error?: string }> =>
      ipcRenderer.invoke('jaz:start-local-backend'),
    getDeviceIdentity: (): Promise<{ device_id: string; public_key: string }> =>
      ipcRenderer.invoke('jaz:get-device-identity'),
    getDeviceMetadata: (): Promise<{
      name: string
      platform: string
      device_family: string
      model_identifier: string
      app_version: string
    }> => ipcRenderer.invoke('jaz:get-device-metadata'),
    configureThreadNotifications: (config: ThreadNotificationConfig): Promise<boolean> =>
      ipcRenderer.invoke('jaz:configure-thread-notifications', config),
    notifications: {
      test: () => ipcRenderer.invoke('jaz:notifications:test'),
      openSettings: () => ipcRenderer.invoke('jaz:notifications:open-settings'),
    } satisfies NotificationsAPI,
    getUpdateStatus: (): Promise<UpdateStatus> => ipcRenderer.invoke('jaz:get-update-status'),
    installUpdate: (): Promise<{ ok: boolean; error?: string }> =>
      ipcRenderer.invoke('jaz:install-update'),
    onUpdateStatus: (handler: (status: UpdateStatus) => void): (() => void) => {
      const listener = (_event: unknown, status: unknown): void => {
        if (
          typeof status === 'object' &&
          status !== null &&
          'state' in status &&
          typeof status.state === 'string'
        ) {
          handler(status as UpdateStatus)
        }
      }
      ipcRenderer.on('jaz:update-status', listener)
      return () => ipcRenderer.removeListener('jaz:update-status', listener)
    },
    openBoardWindow: (boardId: string) => ipcRenderer.send('jaz:open-board-window', boardId),
    openExternalURL: (url: string) => ipcRenderer.send('jaz:open-external-url', url),
    captureScreenRect: (rect: {
      x: number
      y: number
      width: number
      height: number
    }): Promise<{ ok: boolean; data?: string; denied?: boolean }> =>
      ipcRenderer.invoke('jaz:capture-screen-rect', rect),
    hideLauncher: () => ipcRenderer.send('jaz:hide-launcher'),
    onLauncherShown: (handler: () => void): (() => void) => {
      const listener = (): void => handler()
      ipcRenderer.on('jaz:launcher-shown', listener)
      return () => ipcRenderer.removeListener('jaz:launcher-shown', listener)
    },
    // Board windows deep-link into the main app instead of navigating themselves.
    openInMain: (path: string) => ipcRenderer.send('jaz:open-in-main', path),
    onOpenRoute: (handler: (path: string) => void): (() => void) => {
      const listener = (_event: unknown, path: unknown): void => {
        if (typeof path === 'string') handler(path)
      }
      ipcRenderer.on('jaz:open-route', listener)
      return () => ipcRenderer.removeListener('jaz:open-route', listener)
    },
    onOpenPreviewURL: (handler: (url: string) => void): (() => void) => {
      const listener = (_event: unknown, url: unknown): void => {
        if (typeof url === 'string') handler(url)
      }
      ipcRenderer.on('jaz:open-preview-url', listener)
      previewURLTargetSubscriptions += 1
      if (previewURLTargetSubscriptions === 1) {
        ipcRenderer.send('jaz:set-preview-url-target-active', true)
      }
      return () => {
        ipcRenderer.removeListener('jaz:open-preview-url', listener)
        previewURLTargetSubscriptions = Math.max(0, previewURLTargetSubscriptions - 1)
        if (previewURLTargetSubscriptions === 0) {
          ipcRenderer.send('jaz:set-preview-url-target-active', false)
        }
      }
    },
    onBrowserNavigation: (handler: (direction: BrowserNavigationDirection) => void): (() => void) => {
      const listener = (_event: unknown, direction: unknown): void => {
        if (direction === 'back' || direction === 'forward') handler(direction)
      }
      ipcRenderer.on(BROWSER_NAVIGATION_CHANNEL, listener)
      return () => ipcRenderer.removeListener(BROWSER_NAVIGATION_CHANNEL, listener)
    },
    onPreviewFindShortcut: (handler: () => void): (() => void) => {
      const listener = (): void => handler()
      ipcRenderer.on(PREVIEW_FIND_SHORTCUT_CHANNEL, listener)
      return () => ipcRenderer.removeListener(PREVIEW_FIND_SHORTCUT_CHANNEL, listener)
    },
  })
}
