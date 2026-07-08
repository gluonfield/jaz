export type PreviewFindAction = 'clearSelection' | 'keepSelection' | 'activateSelection'

export type PreviewWebviewElement = HTMLElement & {
  src: string
  canGoBack: () => boolean
  canGoForward: () => boolean
  getURL: () => string
  goBack: () => void
  goForward: () => void
  reload: () => void
  findInPage: (text: string, options?: { forward?: boolean; findNext?: boolean; matchCase?: boolean }) => number
  stopFindInPage: (action: PreviewFindAction) => void
  executeJavaScript: <T = unknown>(code: string, userGesture?: boolean) => Promise<T>
  capturePage: () => Promise<{ toDataURL: () => string }>
}

export type PreviewNavigationEvent = Event & {
  url?: string
  validatedURL?: string
  isMainFrame?: boolean
  errorDescription?: string
  errorCode?: number
}

export type PreviewFindResultEvent = Event & {
  result?: {
    activeMatchOrdinal?: number
    matches?: number
  }
}

export function isPreviewWebviewPending(error: unknown): boolean {
  const message = previewWebviewErrorMessage(error)
  return message.includes('WebView must be attached') || message.includes('dom-ready')
}

export function previewWebviewErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
