import type { CloudUser, ServerCapabilities } from '@/lib/api/serverTypes'
import { useCloudAccount, type CloudAccountStatus } from '@/lib/cloudAccount'
import { useConnection } from '@/lib/connection'

export const DEFAULT_CLOUD_URL = 'https://jaz-cloud-staging.camadvanced.com'

export interface ActiveCloudSession {
  active: boolean
  source: 'primary' | 'attached' | null
  status: CloudAccountStatus
  url: string
  user: CloudUser | null
  error: string | null
  oidcLoginUrl: string
  capabilities: ServerCapabilities
  lastSyncAt: string
}

// Unified view of whichever Jaz Cloud session is active — either the primary
// backend (Cloud-only / Web) or the local Go backend's connected Cloud mirror.
export function useCloudSession(): ActiveCloudSession {
  const primary = useConnection()
  const attached = useCloudAccount()

  if (primary.mode === 'cloud') {
    const status: CloudAccountStatus =
      primary.status === 'connected'
        ? 'connected'
        : primary.status === 'checking' || primary.status === 'reconnecting'
          ? 'checking'
          : 'disconnected'
    return {
      active: primary.status === 'connected',
      source: 'primary',
      status,
      url: primary.url,
      user: primary.user,
      error: primary.error,
      oidcLoginUrl: primary.oidcLoginUrl,
      capabilities: primary.capabilities,
      lastSyncAt: '',
    }
  }

  return {
    active: attached.status === 'connected',
    source: attached.status === 'connected' || attached.url ? 'attached' : null,
    status: attached.status,
    url: attached.url,
    user: attached.user,
    error: attached.error,
    oidcLoginUrl: attached.oidcLoginUrl,
    capabilities: attached.capabilities,
    lastSyncAt: attached.lastSyncAt,
  }
}

export function useHasCloud(): boolean {
  return useCloudSession().active
}
