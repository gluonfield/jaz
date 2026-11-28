import { useQuery } from '@tanstack/react-query'
import { orgIdentityQuery } from '@/lib/api/cloudOrg'
import type { CloudUser, ServerCapabilities } from '@/lib/api/serverTypes'
import { clientRuntime } from '@/lib/clientRuntime'
import {
  LOCAL_WORKSPACE_ID,
  useCloudAccount,
  type CloudAccountStatus,
  type CloudWorkspaceSummary,
} from '@/lib/cloudAccount'
import { activeCloudWorkspace } from '@/lib/cloudWorkspaceDisplay'
import { useConnection } from '@/lib/connection'

export interface ActiveCloudSession {
  active: boolean
  source: 'primary' | 'attached' | null
  status: CloudAccountStatus
  activeWorkspaceId: string
  workspaces: CloudWorkspaceSummary[]
  url: string
  user: CloudUser | null
  error: string | null
  oidcLoginUrl: string
  capabilities: ServerCapabilities
  lastSyncAt: string
}

// Unified view of whichever Jaz Cloud session is active — either the primary
// backend (Cloud-only / Web) or the local Go backend's active Cloud workspace.
// The active workspace carries the org's live name and icon.
export function useCloudSession(): ActiveCloudSession {
  const session = useConnectedCloudSession()
  const identity = useQuery({ ...orgIdentityQuery, enabled: session.active }).data
  if (!identity) return session
  return {
    ...session,
    workspaces: session.workspaces.map((ws) =>
      ws.id === session.activeWorkspaceId && ws.org_id === identity.org_id
        ? { ...ws, org_name: identity.org_name || ws.org_name, org_icon: identity.org_icon }
        : ws,
    ),
  }
}

function useConnectedCloudSession(): ActiveCloudSession {
  const primary = useConnection()
  const attached = useCloudAccount()

  if (primary.mode === 'cloud' || clientRuntime.capabilities?.localBackend === false) {
    const isPrimaryCloud = primary.mode === 'cloud'
    const primaryPresent = isPrimaryCloud && (primary.status === 'connected' || primary.status === 'reconnecting')
    const status: CloudAccountStatus = !isPrimaryCloud
      ? 'disconnected'
      : primary.status === 'connected'
        ? 'connected'
        : primary.status === 'checking' || primary.status === 'reconnecting'
          ? 'checking'
          : 'disconnected'
    const primaryWorkspace: CloudWorkspaceSummary | null =
      primaryPresent && primary.url
        ? {
            id: primary.user?.org_id || primary.url,
            url: primary.url,
            org_id: primary.user?.org_id,
            org_name: primary.user?.org_name,
            email: primary.user?.email,
            display_name: primary.user?.display_name,
            role: primary.user?.role,
            oidc_login_url: primary.oidcLoginUrl || undefined,
            capabilities: primary.capabilities,
          }
        : null
    return {
      active: primaryPresent,
      source: 'primary',
      status,
      activeWorkspaceId: primaryWorkspace?.id ?? '',
      workspaces: primaryWorkspace ? [primaryWorkspace] : [],
      url: isPrimaryCloud ? primary.url : clientRuntime.cloudUrl,
      user: isPrimaryCloud ? primary.user : null,
      error: isPrimaryCloud ? primary.error : null,
      oidcLoginUrl: isPrimaryCloud ? primary.oidcLoginUrl : '',
      capabilities: isPrimaryCloud ? primary.capabilities : {},
      lastSyncAt: '',
    }
  }

  const attachedActive =
    attached.status === 'connected' &&
    attached.activeWorkspaceId !== LOCAL_WORKSPACE_ID &&
    attached.workspaces.length > 0

  return {
    active: attachedActive,
    source: attachedActive || attached.workspaces.length > 0 || attached.url ? 'attached' : null,
    status: attached.status,
    activeWorkspaceId: attached.activeWorkspaceId,
    workspaces: attached.workspaces,
    url: attached.url,
    user: attached.user,
    error: attached.error,
    oidcLoginUrl: attached.oidcLoginUrl,
    capabilities: attached.capabilities,
    lastSyncAt: attached.lastSyncAt,
  }
}

export function useHasCloud(): boolean {
  return Boolean(activeCloudWorkspace(useCloudSession()))
}

export function useIsCloudOnly(): boolean {
  return useConnection().mode === 'cloud' || clientRuntime.capabilities?.localBackend === false
}

export function useIsCloudAdmin(): boolean {
  return useCloudSession().user?.role === 'admin'
}

export function isCloudViewerRole(role?: string | null): boolean {
  return (role ?? '').trim().toLowerCase() === 'viewer'
}

export function useIsCloudViewer(): boolean {
  const cloud = useCloudSession()
  return cloud.active && isCloudViewerRole(cloud.user?.role)
}

export function useCloudCatalogScope() {
  const cloud = useCloudSession()
  return {
    account: JSON.stringify([cloud.source, cloud.url, cloud.activeWorkspaceId, cloud.user?.id]),
    workspaceID: cloud.source === 'attached' ? cloud.activeWorkspaceId : '',
  }
}
