import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import type { ACPAuthLogin } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { request } from '@/lib/api/client'
import { cloudWorkspaceHeaders } from '@/lib/api/cloudScope'

export type CloudACPProfileStatus = 'connected' | 'refreshing' | 'needs_auth' | 'error'

export interface CloudACPAgentSpec {
  agent: string
  display_name: string
  profile_id: string
  provider: string
  default_model: string
  models: string[]
  efforts?: string[]
  default_effort: string
  executable?: boolean
  credential_kind: 'oauth_bundle' | 'api_key'
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

const profilesPath = '/v1/acp/profiles'
const loginPath = profilesPath + '/codex/login'

export async function listCloudACPProfiles(signal?: AbortSignal, workspaceID?: string): Promise<CloudACPProfilesResponse> {
  try {
    const res = await request<{
      profiles?: CloudACPProfile[] | null
      supported_agents?: CloudACPAgentSpec[] | null
    }>(profilesPath, { signal, headers: cloudWorkspaceHeaders(workspaceID) })
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

export const cloudACPProfilesQuery = (account: string, workspaceID?: string) => queryOptions({
  queryKey: keys.cloudACPProfiles(account),
  queryFn: ({ signal }) => listCloudACPProfiles(signal, workspaceID),
  staleTime: 30_000,
  retry: false,
})

export async function saveCloudACPKey(agent: string, apiKey: string, workspaceID: string, signal: AbortSignal) {
  await request(`${profilesPath}/${encodeURIComponent(agent)}`, {
    method: 'PUT',
    headers: { ...cloudWorkspaceHeaders(workspaceID), 'Content-Type': 'application/json' },
    body: JSON.stringify({ api_key: apiKey }),
    signal,
  })
}

export async function deleteCloudACPProfile(agent: string, workspaceID: string, signal?: AbortSignal) {
  return request<{ deleted: boolean; agent: string }>(`${profilesPath}/${encodeURIComponent(agent)}`, {
    method: 'DELETE',
    headers: cloudWorkspaceHeaders(workspaceID),
    signal,
  })
}

export async function startCloudACPLogin(workspaceID: string, signal?: AbortSignal): Promise<CloudACPLoginSession> {
  const res = await request<{ session: CloudACPLoginSession }>(loginPath, {
    method: 'POST',
    headers: cloudWorkspaceHeaders(workspaceID),
    signal,
  })
  return res.session
}

export async function getCloudACPLogin(sessionId: string, workspaceID: string, signal?: AbortSignal): Promise<CloudACPLoginSession> {
  const res = await request<{ session: CloudACPLoginSession }>(
    `${loginPath}/${encodeURIComponent(sessionId)}`,
    { signal, headers: cloudWorkspaceHeaders(workspaceID) },
  )
  return res.session
}

export function toACPAuthLogin(session: CloudACPLoginSession): ACPAuthLogin {
  const status: ACPAuthLogin['status'] =
    session.status === 'completed'
      ? 'succeeded'
      : session.status === 'failed' || session.status === 'expired'
        ? 'failed'
        : 'running'
  return {
    id: session.id,
    agent: session.agent,
    status,
    auth_url: session.verification_url,
    auth_code: session.user_code,
    error: session.error || (session.status === 'expired' ? 'Sign-in timed out. Start sign-in again.' : undefined),
    started_at: '',
  }
}
