import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiWebSocketUrl, apiWebSocketProtocols } from '@/lib/api/client'
import { isCloudThread } from '@/lib/api/cloudClient'
import { computerSettingsQuery } from '@/lib/api/settings'
import { connectComputer } from '@/lib/computerConnection'

export function useComputerControl(sessionId: string, enabled = true): void {
  const active = enabled && !isCloudThread(sessionId)
  const settings = useQuery({ ...computerSettingsQuery, enabled: active })
  useEffect(() => {
    const host = window.jaz?.computer
    if (active && host && settings.data?.enabled) {
      return connectComputer(apiWebSocketUrl('/v1/sessions/' + encodeURIComponent(sessionId) + '/computer'), host, sessionId, apiWebSocketProtocols())
    }
  }, [active, settings.data?.enabled, sessionId])
}
