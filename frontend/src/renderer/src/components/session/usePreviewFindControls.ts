import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { clientRuntime } from '@/lib/clientRuntime'
import {
  isPreviewWebviewPending,
  previewWebviewErrorMessage,
  type PreviewFindAction,
  type PreviewFindResultEvent,
  type PreviewWebviewElement,
} from './previewWebview'

export function usePreviewFindControls({
  webview,
  webviewReady,
  canUseWebview,
  onError,
}: {
  webview: PreviewWebviewElement | null
  webviewReady: boolean
  canUseWebview: boolean
  onError: (message: string) => void
}) {
  const inputRef = useRef<HTMLInputElement | null>(null)
  const webviewRef = useRef<PreviewWebviewElement | null>(null)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const [matches, setMatches] = useState(0)

  useEffect(() => {
    webviewRef.current = webview
    setActive(0)
    setMatches(0)
  }, [webview])

  const stopWebviewFind = useCallback((action: PreviewFindAction = 'clearSelection', reportError = true) => {
    try {
      webviewRef.current?.stopFindInPage(action)
    } catch (err) {
      if (reportError && !isPreviewWebviewPending(err)) onError(previewWebviewErrorMessage(err))
    }
  }, [onError])

  const clear = useCallback(() => {
    stopWebviewFind()
    setActive(0)
    setMatches(0)
  }, [stopWebviewFind])

  const openFind = useCallback(() => {
    if (!canUseWebview) return
    setOpen(true)
    requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    })
  }, [canUseWebview])

  const close = useCallback(() => {
    setOpen(false)
    clear()
    requestAnimationFrame(() => webviewRef.current?.focus())
  }, [clear])

  const findAgain = useCallback((forward: boolean) => {
    if (!query || !webviewReady) return
    try {
      webviewRef.current?.findInPage(query, { forward, findNext: true })
    } catch (err) {
      if (!isPreviewWebviewPending(err)) onError(previewWebviewErrorMessage(err))
    }
  }, [onError, query, webviewReady])

  const handleKeyDownCapture = useCallback((event: KeyboardEvent<HTMLElement>) => {
    if (!canUseWebview || event.defaultPrevented) return
    if (!(event.metaKey || event.ctrlKey) || event.shiftKey || event.altKey) return
    if (event.key.toLowerCase() !== 'f') return
    event.preventDefault()
    event.stopPropagation()
    openFind()
  }, [canUseWebview, openFind])

  useEffect(() => {
    if (!webview) return
    const found = (event: PreviewFindResultEvent) => {
      setActive(event.result?.activeMatchOrdinal ?? 0)
      setMatches(event.result?.matches ?? 0)
    }
    webview.addEventListener('found-in-page', found as EventListener)
    return () => webview.removeEventListener('found-in-page', found as EventListener)
  }, [webview])

  useEffect(() => {
    if (!open || !webviewReady) return
    if (!query) {
      clear()
      return
    }
    try {
      webviewRef.current?.findInPage(query)
    } catch (err) {
      if (!isPreviewWebviewPending(err)) onError(previewWebviewErrorMessage(err))
    }
  }, [clear, onError, open, query, webviewReady])

  useEffect(() => clientRuntime.onPreviewFindShortcut?.(openFind), [openFind])

  useEffect(() => () => stopWebviewFind('clearSelection', false), [stopWebviewFind])

  return {
    active,
    close,
    findAgain,
    handleKeyDownCapture,
    inputRef,
    matches,
    open,
    query,
    setQuery,
  }
}

export type PreviewFindControls = ReturnType<typeof usePreviewFindControls>
