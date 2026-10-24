import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiWebSocketUrl, apiWebSocketProtocols } from '@/lib/api/client'
import { computerSettingsQuery } from '@/lib/api/settings'
import { connectComputer } from '@/lib/computerConnection'

export function useComputerControl(sessionId: string): void {
  const settings = useQuery(computerSettingsQuery)
  useEffect(() => {
    const host = window.jaz?.computer
    if (host && settings.data?.enabled) {
      return connectComputer(apiWebSocketUrl('/v1/sessions/' + encodeURIComponent(sessionId) + '/computer'), host, sessionId, apiWebSocketProtocols())
    }
  }, [settings.data?.enabled, sessionId])
}
