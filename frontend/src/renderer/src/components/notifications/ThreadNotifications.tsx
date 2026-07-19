import { useEffect } from 'react'
import { apiAuthToken } from '@/lib/api/client'
import { clientRuntime } from '@/lib/clientRuntime'
import { useConnection } from '@/lib/connection'
import { useThreadNotificationsEnabled } from '@/lib/notificationSettings'

export function ThreadNotifications() {
  const configure = clientRuntime.configureThreadNotifications
  const [enabled] = useThreadNotificationsEnabled()
  const { url } = useConnection()
  const token = apiAuthToken(url)

  useEffect(() => {
    if (!configure) return
    void configure(enabled ? { enabled: true, baseUrl: url, token } : { enabled: false })
  }, [configure, enabled, token, url])

  return null
}
