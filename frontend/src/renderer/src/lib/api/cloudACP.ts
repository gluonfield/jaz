import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'
import { del, get, post } from './client'

export type CloudACPProfileStatus = 'connected' | 'refreshing' | 'needs_auth' | 'error'

export interface CloudACPAgentSpec {
  agent: string
  display_name: string
  profile_id: string
  provider: string
  default_model: string
  models: string[]
  default_effort: string
  supports_web_auth: boolean
  web_auth_kind?: string
}

export interface CloudACPProfile {
  agent: string
  profile_id: string
  display_name: string
  status: CloudACPProfileStatus
  account_label?: string
  version: number
  token_expires_at?: string | null
  last_refreshed_at?: string | null
  last_error?: string
  updated_at: string
}

export interface CloudACPProfilesResponse {
  profiles: CloudACPProfile[]
  supported_agents: CloudACPAgentSpec[]
}

export type CloudACPLoginStatus = 'pending' | 'awaiting_user' | 'completed' | 'failed' | 'expired'

export interface CloudACPLoginSession {
  id: string
  agent: string
  status: CloudACPLoginStatus
  verification_url?: string
  user_code?: string
  instructions?: string
  error?: string
  expires_at: string
}

export interface SyncLocalACPProfileResult {
  profile: CloudACPProfile
  cleared_local?: boolean
  owned_by_jaz?: boolean
}

export async function listCloudACPProfiles(signal?: AbortSignal): Promise<CloudACPProfilesResponse> {
  try {
    const res = await get<{
      profiles?: CloudACPProfile[] | null
      supported_agents?: CloudACPAgentSpec[] | null
    }>('/v1/acp/profiles', { signal })
    return {
      profiles: res.profiles ?? [],
      supported_agents: res.supported_agents ?? [],
    }
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return { profiles: [], supported_agents: [] }
    }
    throw error
  }
}

export const cloudACPProfilesQuery = queryOptions({
  queryKey: keys.cloudACPProfiles,
  queryFn: ({ signal }) => listCloudACPProfiles(signal),
  staleTime: 30_000,
  retry: false,
})

export async function refreshCloudACPProfile(agent: string, force = true): Promise<{ queued: boolean; profile: CloudACPProfile }> {
  return post<{ queued: boolean; profile: CloudACPProfile }>(
    `/v1/acp/profiles/${encodeURIComponent(agent)}/refresh`,
    { force },
  )
}

export async function deleteCloudACPProfile(agent: string): Promise<{ deleted: boolean; agent: string }> {
  return del<{ deleted: boolean; agent: string }>(`/v1/acp/profiles/${encodeURIComponent(agent)}`)
}

export async function startCloudACPLogin(agent: string): Promise<CloudACPLoginSession> {
  const res = await post<{ session: CloudACPLoginSession }>(`/v1/acp/profiles/${encodeURIComponent(agent)}/login`, {})
  return res.session
}

export async function getCloudACPLogin(agent: string, sessionId: string, signal?: AbortSignal): Promise<CloudACPLoginSession> {
  const res = await get<{ session: CloudACPLoginSession }>(
    `/v1/acp/profiles/${encodeURIComponent(agent)}/login/${encodeURIComponent(sessionId)}`,
    { signal },
  )
  return res.session
}

export async function syncLocalACPProfileToCloud(
  agent: string,
  options: { clearLocal?: boolean; accountLabel?: string } = {},
): Promise<SyncLocalACPProfileResult> {
  return post<SyncLocalACPProfileResult>(
    `/v1/cloud/acp-profiles/${encodeURIComponent(agent)}/sync-local`,
    {
      clear_local: Boolean(options.clearLocal),
      ...(options.accountLabel ? { account_label: options.accountLabel } : {}),
    },
  )
}
