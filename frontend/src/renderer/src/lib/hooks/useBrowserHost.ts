import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { browserSettingsQuery } from '@/lib/api/settings'
import { connectBrowser } from '@/lib/browserConnection'
import { clientRuntime } from '@/lib/clientRuntime'
import type { BrowserSessions } from '@/lib/browserSessions'

export function useBrowserHost(sessions: BrowserSessions): void {
  const { data: settings } = useQuery(browserSettingsQuery)
  useEffect(() => {
    if (!window.jaz?.browserCommand || clientRuntime.windowKind !== 'main' || !settings?.enabled) return
    return connectBrowser('/v1/browser/desktop', (request) => {
      if (request.method !== 'Jaz.connect' || typeof request.params?.session !== 'string' || !request.params.session) {
        throw new Error('Invalid browser connection request')
      }
      sessions.ensure(request.params.session)
      return {}
    })
  }, [sessions, settings?.enabled])
}
