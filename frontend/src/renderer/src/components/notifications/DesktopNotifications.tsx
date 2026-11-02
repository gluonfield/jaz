import { useEffect } from 'react'
import { cachedApiAuthToken } from '@/lib/api/client'
import { clientRuntime } from '@/lib/clientRuntime'
import { ownsConnection, useConnection } from '@/lib/connection'
import { useNotificationMode } from '@/lib/notificationSettings'

export function DesktopNotifications() {
  const configure = clientRuntime.configureNotifications
  const [mode] = useNotificationMode()
  const connection = useConnection()

  useEffect(() => {
    if (!configure || !ownsConnection) return
    const { status, url } = connection
    void configure(mode !== 'off' && (status === 'connected' || status === 'reconnecting')
      ? { mode, baseUrl: url, token: cachedApiAuthToken(url) }
      : { mode: 'off' })
  }, [configure, mode, connection])

  return null
}
