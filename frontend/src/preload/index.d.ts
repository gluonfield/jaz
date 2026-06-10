export {}

declare global {
  interface Window {
    jaz: {
      apiBaseUrl: string
      setNativeTheme: (source: 'light' | 'dark' | 'system') => void
      startLocalBackend: () => Promise<{ ok: boolean; error?: string }>
    }
  }
}
