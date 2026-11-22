import { X } from 'lucide-react'
import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { openExternal } from '@/lib/clientRuntime'
import { googlePasskeyDestination } from '@shared/preview'
import type { PreviewNavigationEvent, PreviewWebviewElement } from '@/components/session/previewWebview'

function pageKey(url: string): string {
  const page = URL.parse(url)
  return page ? page.origin + page.pathname : url
}

export function BrowserPasskeyNotice({
  webview,
  webviewReady,
  url,
  title,
  onOpenImport,
}: {
  webview: PreviewWebviewElement | null
  webviewReady: boolean
  url: string
  title?: string
  onOpenImport: () => void
}) {
  const [notice, setNotice] = useState<{ key: string; dismissed: boolean } | null>(null)
  const pendingIds = useRef(new Set<number>())

  const handleFallback = useEffectEvent((id: number) => {
    if (webviewReady && webview) {
      try {
        if (webview.getWebContentsId() === id) {
          setNotice({ key: pageKey(webview.getURL() || url), dismissed: false })
        }
      } catch {
        pendingIds.current.add(id)
      }
      return
    }
    pendingIds.current.add(id)
  })

  const flushPending = useEffectEvent((readyView: PreviewWebviewElement) => {
    try {
      const matched = pendingIds.current.has(readyView.getWebContentsId())
      pendingIds.current.clear()
      if (matched) {
        setNotice({ key: pageKey(readyView.getURL() || url), dismissed: false })
      }
    } catch {
      // Webview detached before reading its id.
    }
  })

  useEffect(() => {
    const onFallback = window.jaz?.onBrowserPasskeyFallback
    if (!onFallback) {
      return
    }
    return onFallback(handleFallback)
  }, [])

  useEffect(() => {
    if (!webview || !webviewReady) {
      return
    }
    flushPending(webview)
  }, [webview, webviewReady])

  useEffect(() => {
    if (!webview) {
      return
    }
    const clearOnNavigation = (event: PreviewNavigationEvent) => {
      if (event.isMainFrame && !event.isInPlace) {
        pendingIds.current.clear()
        setNotice(null)
      }
    }
    webview.addEventListener('did-start-navigation', clearOnNavigation as EventListener)
    return () => webview.removeEventListener('did-start-navigation', clearOnNavigation as EventListener)
  }, [webview])

  if (!url) {
    return null
  }
  const currentKey = pageKey(url)
  const googleDestination = googlePasskeyDestination(url, title)
  const active = notice?.key === currentKey ? !notice.dismissed : Boolean(googleDestination)
  if (!active) {
    return null
  }
  const destination = googleDestination ?? url
  return (
    <div role="status" className="flex shrink-0 flex-wrap items-center justify-between gap-x-3 gap-y-1.5 border-b border-border bg-surface/80 px-3 py-1.5 text-[12px] text-ink-2">
      <span className="min-w-0 flex-1">
        Passkeys aren&apos;t available in the side browser on macOS yet. Use another sign-in method on this page, or sign in in your browser and import cookies.
      </span>
      <div className="flex shrink-0 items-center gap-1.5">
        {window.jaz?.browserProfiles ? (
          <Button size="sm" variant="secondary" className="h-6 px-2" onClick={onOpenImport}>
            Import cookies
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="secondary"
          className="h-6 px-2"
          title={googleDestination ? 'Start a separate Google sign-in in your default browser' : 'Open this page in your default browser'}
          onClick={() => openExternal(destination)}
        >
          {googleDestination ? 'Open Google sign-in in browser' : 'Open in browser'}
        </Button>
        <IconButton
          size="2xs"
          aria-label="Dismiss passkey notice"
          title="Dismiss"
          onClick={() => setNotice({ key: currentKey, dismissed: true })}
        >
          <X size={12} />
        </IconButton>
      </div>
    </div>
  )
}
