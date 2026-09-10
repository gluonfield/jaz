import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiAuthenticatedWebSocketUrl, post } from '@/lib/api/client'
import { browserSettingsQuery } from '@/lib/api/settings'
import type { BrowserCommand } from '@shared/browserControl'
import { SideBrowser } from '@/lib/sideBrowser'

export function useSideBrowser(sessionId: string, open: (url: string) => void): SideBrowser | undefined {
  const settings = useQuery(browserSettingsQuery)
  const [browser] = useState(() => window.jaz?.browserCommand
    ? new SideBrowser(open, window.jaz.browserCommand, (action, signal) => post(`/v1/sessions/${encodeURIComponent(sessionId)}/browser`, action, signal))
    : undefined)
  const enabled = settings.data?.enabled && settings.data.mode === 'desktop'

  useEffect(() => {
    if (!browser || !enabled) {
      return
    }
    let stopped = false
    let socket: WebSocket
    let retry: ReturnType<typeof setTimeout>
    const connect = () => {
      socket = new WebSocket(apiAuthenticatedWebSocketUrl(`/v1/sessions/${encodeURIComponent(sessionId)}/browser`))
      const current = socket
      current.onmessage = async (event) => {
        if (stopped) {
          return
        }
        let request: BrowserCommand & { id: number }
        try {
          request = JSON.parse(event.data)
        } catch {
          current.close()
          return
        }
        try {
          const result = await browser.call(request)
          if (current.readyState === WebSocket.OPEN) {
            current.send(JSON.stringify({ id: request.id, result }))
          }
        } catch (error) {
          if (current.readyState === WebSocket.OPEN) {
            current.send(JSON.stringify({ id: request.id, error: { code: -32000, message: error instanceof Error ? error.message : String(error) } }))
          }
        }
      }
      current.onclose = () => {
        if (stopped) {
          return
        }
        browser.cancel()
        retry = setTimeout(connect, 2000)
      }
    }
    connect()
    return () => {
      stopped = true
      clearTimeout(retry)
      socket.close()
      browser.dispose()
    }
  }, [browser, enabled, sessionId])

  return enabled ? browser : undefined
}
