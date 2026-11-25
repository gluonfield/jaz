import { claudeVoice } from '@preload/claudeVoiceAPI'
import { CLAUDE_VOICE_ARGUMENT } from '@shared/claudeVoice'
import { installClaudeVoice } from '@preload/claudeVoice'
import { contextBridge, ipcRenderer } from 'electron'
import { inspectDroppedFile } from '@preload/inspectDroppedFile'
import { COMPUTER_CHANNEL, type ComputerAPI } from '@shared/computerControl'
import {
  PERMISSION_GUIDE_CHANNEL,
  SYSTEM_PERMISSIONS_CHANNEL,
  type PermissionGuideAPI,
  type SystemPermissionsAPI,
} from '@shared/systemPermissions'
import { SECONDARY_WINDOW_KINDS } from '@shared/windowKind'
import { browserPasswords } from '@preload/browserPasswords'
import { browserDownloads } from '@preload/browserDownloads'
import { installBrowserPasswordCapture } from '@preload/browser'
import {
  BROWSER_PASSKEY_FALLBACK_ARGUMENT,
  BROWSER_PASSKEY_FALLBACK_CHANNEL,
  BROWSER_PRELOAD_ARGUMENT,
} from '@shared/preview'
import { BROWSER_COMMAND_CHANNEL, type BrowserCommandRequest } from '@shared/browserControl'
import { BROWSER_PROFILE_CHANNELS, type BrowserProfileAPI } from '@shared/browserProfile'
import {
  BROWSER_NAVIGATION_CHANNEL,
  type BrowserNavigationDirection,
} from '../shared/browserNavigation'
import { PREVIEW_FIND_SHORTCUT_CHANNEL } from '../shared/previewFind'
import { BROWSER_CLOSE_SHORTCUT_CHANNEL } from '@shared/browserCloseShortcut'
import { ISLAND_CHANNEL, type IslandAPI, type NotchAccess, type NotificationConfig } from '../shared/notifications'
import type { UpdateStatus } from '../shared/update'
import type { DictationAPI, DictationEvent } from '../shared/dictation'
import type { VoiceCommand, VoiceOverlayAPI, VoiceOverlayState } from '@shared/voice'
import { CONNECTION_SESSION_CHANNEL, LOCAL_SESSION_CHANNEL, type ConnectionSessionAPI, type SessionDocument, type SessionSnapshot } from '@shared/connectionSession'
import { readNodeCloudConfig } from '@shared/cloudConfig'
import { LOCAL_BACKEND_TRANSPORT_URL, LOCAL_BACKEND_URL } from '@shared/localBackend'
import { FILE_ACTIONS_CHANNEL, type FileActionsAPI } from '@shared/fileActions'

const cloudConfig = readNodeCloudConfig(process.env)
const apiBaseUrl = process.env['JAZ_API_URL'] ?? (cloudConfig.localEnabled ? LOCAL_BACKEND_URL : cloudConfig.cloudUrl || LOCAL_BACKEND_URL)

const windowKind = SECONDARY_WINDOW_KINDS.find((kind) => process.argv.includes(`--jaz-${kind}-window`)) ?? 'main'
let previewURLTargetSubscriptions = 0
let sessionDocument: number | undefined

function sessionRequest<T>(request: unknown): T {
  const result = ipcRenderer.sendSync(CONNECTION_SESSION_CHANNEL, request) as { value: T; error?: string }
  if (result.error) {
    throw new Error(result.error)
  }
  return result.value
}

const connectionSession: ConnectionSessionAPI = {
  read: () => {
    const result = sessionRequest<SessionDocument>({ operation: 'read' })
    sessionDocument = result.document
    return result
  },
  update: (request) => sessionRequest<SessionSnapshot>(request),
  local: (document, revision) => ipcRenderer.invoke(LOCAL_SESSION_CHANNEL, document, revision),
  subscribe: (handler) => {
    const listener = (_event: unknown, value: { document: number; snapshot: SessionSnapshot }) => {
      if (value.document === sessionDocument) {
        handler(value.snapshot)
      }
    }
    ipcRenderer.on(CONNECTION_SESSION_CHANNEL, listener)
    return () => ipcRenderer.removeListener(CONNECTION_SESSION_CHANNEL, listener)
  },
}

if (process.argv.includes(CLAUDE_VOICE_ARGUMENT)) {
  installClaudeVoice()
} else if (process.argv.includes(BROWSER_PRELOAD_ARGUMENT)) {
  installBrowserPasswordCapture()
} else {
  contextBridge.exposeInMainWorld('jaz', {
    claudeVoice,
    connectionSession,
    browserPasswords,
    browserDownloads,
    browserPasskeyFallback: process.argv.includes(BROWSER_PASSKEY_FALLBACK_ARGUMENT),
    fileActions: {
      canReveal: (backendUrl, sessionId) => ipcRenderer.invoke(FILE_ACTIONS_CHANNEL, 'canReveal', backendUrl, sessionId),
      reveal: (backendUrl, sessionId, path) => ipcRenderer.invoke(FILE_ACTIONS_CHANNEL, 'reveal', backendUrl, sessionId, path),
    } satisfies FileActionsAPI,
    computer: {
      status: () => ipcRenderer.invoke(COMPUTER_CHANNEL + 'status'),
      begin: (id, session) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'begin', id, session),
      call: (id, action) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'call', id, action),
      end: (id) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'end', id),
      cancel: (id) => ipcRenderer.invoke(COMPUTER_CHANNEL + 'cancel', id),
    } satisfies ComputerAPI,
    systemPermissions: {
      status: () => ipcRenderer.invoke(SYSTEM_PERMISSIONS_CHANNEL + 'status'),
      allow: (permission) => ipcRenderer.invoke(SYSTEM_PERMISSIONS_CHANNEL + 'allow', permission),
    } satisfies SystemPermissionsAPI,
    permissionGuide: {
      drag: () => ipcRenderer.send(PERMISSION_GUIDE_CHANNEL + 'drag'),
      close: () => ipcRenderer.send(PERMISSION_GUIDE_CHANNEL + 'close'),
    } satisfies PermissionGuideAPI,
    voiceOverlay: {
      drag: (point) => ipcRenderer.send('jaz:voice:drag', point),
      resize: (height) => ipcRenderer.send('jaz:voice:resize', height),
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
    island: {
      onState: (listener) => {
        const handler = (_event: unknown, state: Parameters<typeof listener>[0]) => listener(state)
        ipcRenderer.on(`${ISLAND_CHANNEL}state`, handler)
        ipcRenderer.send(`${ISLAND_CHANNEL}ready`)
        return () => ipcRenderer.removeListener(`${ISLAND_CHANNEL}state`, handler)
      },
      command: (command) => ipcRenderer.send(`${ISLAND_CHANNEL}command`, command),
    } satisfies IslandAPI,
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
    encryptCredential: (value: string): string => {
      const result = ipcRenderer.sendSync('jaz:credential-encryption', 'encrypt', value) as { value?: string; error?: string }
      if (result.error || !result.value) {
        throw new Error(result.error || 'Credential encryption failed')
      }
      return result.value
    },
    decryptCredential: (value: string): string => {
      const result = ipcRenderer.sendSync('jaz:credential-encryption', 'decrypt', value) as { value?: string; error?: string }
      if (result.error || result.value === undefined) {
        throw new Error(result.error || 'Credential decryption failed')
      }
      return result.value
    },
    apiBaseUrl,
    cloudUrl: cloudConfig.cloudUrl,
    localEnabled: cloudConfig.localEnabled,
    localBackendTransport: LOCAL_BACKEND_TRANSPORT_URL,
    windowKind,
    inspectDroppedFile,
    setNativeTheme: (source: 'light' | 'dark' | 'system') =>
      ipcRenderer.send('jaz:set-native-theme', source),
    setWindowButtonsVisible: (visible: boolean) => ipcRenderer.send('jaz:set-window-buttons-visible', visible),
    startLocalBackend: (): Promise<{ ok: boolean; url?: string; key?: string; error?: string }> =>
      ipcRenderer.invoke('jaz:start-local-backend'),
    beginOidcSignIn: (): Promise<{ redirectUri?: string } | void> => ipcRenderer.invoke('jaz:oidc-begin'),
    onOidcCallback: (handler: (url: string) => void): (() => void) => {
      const listener = (_event: unknown, url: unknown): void => {
        if (typeof url === 'string') handler(url)
      }
      ipcRenderer.on('jaz:oidc-callback', listener)
      return () => ipcRenderer.removeListener('jaz:oidc-callback', listener)
    },
    pickDirectory: (): Promise<string | null> => ipcRenderer.invoke('jaz:pick-directory'),
    configureNotifications: (config: NotificationConfig): Promise<boolean> =>
      ipcRenderer.invoke('jaz:configure-notifications', config),
    notchAccess: (): Promise<NotchAccess> => ipcRenderer.invoke('jaz:notifications:notch-access'),
    shareFocusStatus: (): Promise<NotchAccess> => ipcRenderer.invoke('jaz:notifications:share-focus'),
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
    onBrowserCloseShortcut: (handler: () => void): (() => void) => {
      const listener = () => handler()
      ipcRenderer.on(BROWSER_CLOSE_SHORTCUT_CHANNEL, listener)
      return () => ipcRenderer.removeListener(BROWSER_CLOSE_SHORTCUT_CHANNEL, listener)
    },
    onPreviewFindShortcut: (handler: () => void): (() => void) => {
      const listener = (): void => handler()
      ipcRenderer.on(PREVIEW_FIND_SHORTCUT_CHANNEL, listener)
      return () => ipcRenderer.removeListener(PREVIEW_FIND_SHORTCUT_CHANNEL, listener)
    },
    onBrowserPasskeyFallback: (handler: (webContentsId: number) => void): (() => void) => {
      const listener = (_event: unknown, webContentsId: unknown): void => {
        if (typeof webContentsId === 'number') {
          handler(webContentsId)
        }
      }
      ipcRenderer.on(BROWSER_PASSKEY_FALLBACK_CHANNEL, listener)
      return () => ipcRenderer.removeListener(BROWSER_PASSKEY_FALLBACK_CHANNEL, listener)
    },
  })
}
