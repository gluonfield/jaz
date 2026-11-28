import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import type { ACPAuthLoginChallenge } from '@/lib/api/types'
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
  web_auth_kind?: 'device_code' | 'oauth_code' | 'oauth_redirect'
}

export interface CloudACPProfile {
  agent: string
  profile_id: string
  display_name: string
  credential_kind: CloudACPAgentSpec['credential_kind']
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
const profilePath = (agent: string) => `${profilesPath}/${encodeURIComponent(agent)}`
const loginPath = (agent: string, sessionId: string) => `${profilePath(agent)}/login/${encodeURIComponent(sessionId)}`

interface CloudACPLoginOptions {
  gcp_project?: string
  gcp_location?: string
}

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
  await request(profilePath(agent), {
    method: 'PUT',
    headers: { ...cloudWorkspaceHeaders(workspaceID), 'Content-Type': 'application/json' },
    body: JSON.stringify({ api_key: apiKey }),
    signal,
  })
}

export async function deleteCloudACPProfile(agent: string, workspaceID: string, signal?: AbortSignal) {
  return request<{ deleted: boolean; agent: string }>(profilePath(agent), {
    method: 'DELETE',
    headers: cloudWorkspaceHeaders(workspaceID),
    signal,
  })
}

function postCloudACP<T>(path: string, workspaceID: string, signal: AbortSignal, body?: object) {
  return request<T>(path, {
    method: 'POST',
    headers: { ...cloudWorkspaceHeaders(workspaceID), 'Content-Type': 'application/json' },
    body: body && JSON.stringify(body),
    signal,
  })
}

export async function startCloudACPLogin(agent: string, workspaceID: string, signal: AbortSignal, options?: CloudACPLoginOptions) {
  const res = await postCloudACP<{ session: CloudACPLoginSession }>(`${profilePath(agent)}/login`, workspaceID, signal, options)
  return res.session
}

export async function getCloudACPLogin(agent: string, sessionId: string, workspaceID: string, signal: AbortSignal) {
  const res = await request<{ session: CloudACPLoginSession }>(loginPath(agent, sessionId), {
    signal,
    headers: cloudWorkspaceHeaders(workspaceID),
  })
  return res.session
}

export async function completeCloudACPLogin(agent: string, sessionId: string, code: string, workspaceID: string, signal: AbortSignal) {
  const res = await postCloudACP<{ session: CloudACPLoginSession }>(`${loginPath(agent, sessionId)}/complete`, workspaceID, signal, { code })
  return res.session
}

export function refreshCloudACPProfile(agent: string, workspaceID: string, signal: AbortSignal) {
  return postCloudACP<{ profile: CloudACPProfile }>(`${profilePath(agent)}/refresh`, workspaceID, signal, { force: true })
}

export function toACPAuthLogin(session: CloudACPLoginSession): ACPAuthLoginChallenge {
  const status: ACPAuthLoginChallenge['status'] =
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
  }
}
