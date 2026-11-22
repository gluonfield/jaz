import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiWebSocketUrl, apiWebSocketProtocols } from '@/lib/api/client'
import { computerSettingsQuery } from '@/lib/api/settings'
import { connectComputer } from '@/lib/computerConnection'
import { useIsCloud } from '@/lib/connection'

export function useComputerControl(sessionId: string): void {
  const isCloud = useIsCloud()
  const settings = useQuery({ ...computerSettingsQuery, enabled: !isCloud })
  useEffect(() => {
    const host = window.jaz?.computer
    if (!isCloud && host && settings.data?.enabled) {
      return connectComputer(apiWebSocketUrl('/v1/sessions/' + encodeURIComponent(sessionId) + '/computer'), host, sessionId, apiWebSocketProtocols())
    }
  }, [isCloud, settings.data?.enabled, sessionId])
}
