import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiWebSocketUrl, apiWebSocketProtocols, isCloudApiSession } from '@/lib/api/client'
import { computerSettingsQuery } from '@/lib/api/settings'
import { connectComputer } from '@/lib/computerConnection'

export function useComputerControl(sessionId: string, enabled = true): void {
  const active = enabled && !isCloudApiSession()
  const settings = useQuery({ ...computerSettingsQuery, enabled: active })
  useEffect(() => {
    const host = window.jaz?.computer
    if (active && host && settings.data?.enabled) {
      return connectComputer(apiWebSocketUrl('/v1/sessions/' + encodeURIComponent(sessionId) + '/computer'), host, sessionId, apiWebSocketProtocols())
    }
  }, [active, settings.data?.enabled, sessionId])
}
