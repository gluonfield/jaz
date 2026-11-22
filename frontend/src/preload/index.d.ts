import type { ClaudeVoiceAPI } from '@shared/claudeVoice'

export {}

import type { ComputerAPI } from '@shared/computerControl'
import type { PermissionGuideAPI, SystemPermissionsAPI } from '@shared/systemPermissions'
import type { WindowKind } from '@shared/windowKind'

import type { BrowserNavigationDirection } from '../shared/browserNavigation'
import type { IslandAPI, NotchAccess, NotificationConfig } from '../shared/notifications'
import type { UpdateStatus } from '../shared/update'
import type { DictationAPI } from '../shared/dictation'
import type { BrowserCommandRequest } from '@shared/browserControl'
import type { BrowserProfileAPI } from '@shared/browserProfile'
import type { BrowserPasswordAPI } from '@shared/browserPasswords'
import type { BrowserDownloadAPI } from '@shared/browserDownloads'
import type { VoiceOverlayAPI } from '@shared/voice'
import type { ConnectionSessionAPI } from '@shared/connectionSession'
import type { FileActionsAPI } from '@shared/fileActions'

declare global {
  interface Window {
    jaz?: {
      claudeVoice: ClaudeVoiceAPI
      connectionSession: ConnectionSessionAPI
      fileActions: FileActionsAPI
      computer: ComputerAPI
      systemPermissions: SystemPermissionsAPI
      permissionGuide: PermissionGuideAPI
      voiceOverlay: VoiceOverlayAPI
      island: IslandAPI
      dictation: DictationAPI
      browserProfiles: BrowserProfileAPI
      browserPasswords: BrowserPasswordAPI
      browserDownloads: BrowserDownloadAPI
      browserPasskeyFallback: boolean
      browserCommand: (request: BrowserCommandRequest) => Promise<unknown>
      encryptCredential: (value: string) => string
      decryptCredential: (value: string) => string
      apiBaseUrl: string
      cloudUrl?: string
      localEnabled?: boolean
      localBackendTransport?: string
      windowKind: WindowKind
      pathForFile: (file: File) => string
      setNativeTheme: (source: 'light' | 'dark' | 'system') => void
      setWindowButtonsVisible: (visible: boolean) => void
      startLocalBackend: () => Promise<{ ok: boolean; url?: string; key?: string; error?: string }>
      beginOidcSignIn?: () => Promise<void>
      onOidcCallback?: (handler: (url: string) => void) => () => void
      pickDirectory?: () => Promise<string | null>
      configureNotifications: (config: NotificationConfig) => Promise<boolean>
      notchAccess: () => Promise<NotchAccess>
      shareFocusStatus: () => Promise<NotchAccess>
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
      onBrowserCloseShortcut: (handler: () => void) => () => void
      onPreviewFindShortcut: (handler: () => void) => () => void
    }
  }
}
