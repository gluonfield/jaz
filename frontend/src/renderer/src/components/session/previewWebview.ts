export type PreviewWebviewElement = HTMLElement & {
  src: string
  canGoBack: () => boolean
  canGoForward: () => boolean
  getURL: () => string
  goBack: () => void
  goForward: () => void
  reload: () => void
}

export type PreviewNavigationEvent = Event & {
  url?: string
  validatedURL?: string
  isMainFrame?: boolean
  errorDescription?: string
  errorCode?: number
}
