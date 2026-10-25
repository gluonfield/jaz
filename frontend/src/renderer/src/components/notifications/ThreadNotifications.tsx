import { useEffect } from 'react'
import { apiAuthToken } from '@/lib/api/client'
import { clientRuntime } from '@/lib/clientRuntime'
import { useConnection } from '@/lib/connection'
import { useThreadNotificationsEnabled } from '@/lib/notificationSettings'

export function ThreadNotifications() {
  const configure = clientRuntime.configureThreadNotifications
  const [enabled] = useThreadNotificationsEnabled()
  const connection = useConnection()

  useEffect(() => {
    if (!configure) return
    const { status, url } = connection
    if (enabled && (status === 'connected' || status === 'reconnecting')) {
      try {
        void configure({ enabled: true, baseUrl: url, token: apiAuthToken(url) })
        return
      } catch {
        // A locked optional credential must not unmount the app.
      }
    }
    void configure({ enabled: false })
  }, [configure, enabled, connection])

  return null
}
