import { useEffect } from 'react'
import { cachedApiAuthToken } from '@/lib/api/client'
import { clientRuntime } from '@/lib/clientRuntime'
import { ownsConnection, useConnection } from '@/lib/connection'
import { useThreadNotificationsEnabled } from '@/lib/notificationSettings'

export function ThreadNotifications() {
  const configure = clientRuntime.configureThreadNotifications
  const [enabled] = useThreadNotificationsEnabled()
  const connection = useConnection()

  useEffect(() => {
    if (!configure || !ownsConnection) return
    const { status, url } = connection
    void configure(enabled && (status === 'connected' || status === 'reconnecting')
      ? { enabled: true, baseUrl: url, token: cachedApiAuthToken(url) }
      : { enabled: false })

  }, [configure, enabled, connection])

  return null
}
