export {}

declare global {
  interface Window {
    jaz: {
      apiBaseUrl: string
      windowKind: 'main' | 'board'
      setNativeTheme: (source: 'light' | 'dark' | 'system') => void
      startLocalBackend: () => Promise<{ ok: boolean; error?: string }>
      openBoardWindow: (boardId: string) => void
      openInMain: (path: string) => void
      onOpenRoute: (handler: (path: string) => void) => () => void
    }
  }
}
