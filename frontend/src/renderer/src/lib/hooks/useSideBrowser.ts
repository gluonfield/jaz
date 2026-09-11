import { useEffect, useEffectEvent, useState } from 'react'
import { flushSync } from 'react-dom'
import { useQuery } from '@tanstack/react-query'
import { ApiError, apiAuthenticatedWebSocketUrl, post } from '@/lib/api/client'
import { getSession } from '@/lib/api/sessions'
import { browserSettingsQuery } from '@/lib/api/settings'
import type { BrowserCommand } from '@shared/browserControl'
import { SideBrowser } from '@/lib/sideBrowser'
import { BrowserRetention } from '@/lib/browserRetention'

export function useSideBrowser({ sessionId, open, visible, url, idleMs }: {
  sessionId: string
  open: (url: string) => void
  visible: boolean
  url: string
  idleMs: number
}) {
  const settings = useQuery(browserSettingsQuery)
  const [resident, setResident] = useState(false)
  const [browser] = useState(() => window.jaz?.browserCommand
    ? new SideBrowser((url) => {
      setResident(true)
      open(url)
    }, window.jaz.browserCommand, (action, signal) => post(`/v1/sessions/${encodeURIComponent(sessionId)}/browser`, action, signal))
    : undefined)
  const [retention] = useState(() => new BrowserRetention(async (signal) => {
    try {
      const session = await getSession(sessionId, AbortSignal.any([signal, AbortSignal.timeout(10_000)]))
      return ['idle', 'error', 'interrupted'].includes(session.status) && !session.queued_messages?.length && !session.pending_steer_message
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) return true
      throw error
    }
  }, () => {
    browser?.dispose()
    flushSync(() => setResident(false))
  }, idleMs))
  const enabled = settings.data?.enabled && settings.data.mode === 'desktop'
  useEffect(() => {
    if (visible) setResident(true)
    retention.configure(!visible && resident)
    return () => retention.configure(false)
  }, [retention, visible, resident])

  const execute = useEffectEvent((control: SideBrowser, request: BrowserCommand) => retention.run(async () => {
    if (request.method !== 'Jaz.tab') {
      setResident(true)
      if (url && request.method !== 'Jaz.open') await control.waitForViewport()
    }
    return control.call(request)
  }))

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
          const result = await execute(browser, request)
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

  return { browser: enabled ? browser : undefined, resident }
}
