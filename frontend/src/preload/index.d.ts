export {}

import type { UpdateStatus } from '../shared/update'

declare global {
  interface Window {
    jaz: {
      apiBaseUrl: string
      windowKind: 'main' | 'board'
      setNativeTheme: (source: 'light' | 'dark' | 'system') => void
      startLocalBackend: () => Promise<{ ok: boolean; url?: string; key?: string; error?: string }>
      getUpdateStatus: () => Promise<UpdateStatus>
      installUpdate: () => Promise<{ ok: boolean; error?: string }>
      onUpdateStatus: (handler: (status: UpdateStatus) => void) => () => void
      openBoardWindow: (boardId: string) => void
      openInMain: (path: string) => void
      onOpenSideBrowserURL: (handler: (url: string) => void) => () => void
      onOpenRoute: (handler: (path: string) => void) => () => void
      onOpenPreviewURL: (handler: (url: string) => void) => () => void
    }
  }
}
