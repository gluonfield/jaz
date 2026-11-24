import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import type { ACPAuthLogin } from '@/lib/api/types'
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
  efforts?: string[]
  default_effort: string
  executable?: boolean
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
  viaDesktop?: boolean
}

const DESKTOP_ISOLATED_LOGIN_AGENTS = new Set(['claude', 'kimi'])

export function supportsDesktopCloudLogin(agent: string): boolean {
  return DESKTOP_ISOLATED_LOGIN_AGENTS.has(agent.trim().toLowerCase())
}

function loginBasePath(agent: string, viaDesktop: boolean): string {
  const prefix = viaDesktop ? '/v1/cloud/acp-profiles' : '/v1/acp/profiles'
  return `${prefix}/${encodeURIComponent(agent)}/login`
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

export async function deleteCloudACPProfile(agent: string): Promise<{ deleted: boolean; agent: string }> {
  return del<{ deleted: boolean; agent: string }>(`/v1/acp/profiles/${encodeURIComponent(agent)}`)
}

export async function startCloudACPLogin(agent: string, viaDesktop = false): Promise<CloudACPLoginSession> {
  const res = await post<{ session: CloudACPLoginSession }>(loginBasePath(agent, viaDesktop), {})
  return { ...res.session, viaDesktop }
}

export async function getCloudACPLogin(
  agent: string,
  sessionId: string,
  viaDesktop = false,
  signal?: AbortSignal,
): Promise<CloudACPLoginSession> {
  const res = await get<{ session: CloudACPLoginSession }>(
    `${loginBasePath(agent, viaDesktop)}/${encodeURIComponent(sessionId)}`,
    { signal },
  )
  return { ...res.session, viaDesktop }
}

export function toACPAuthLogin(session?: CloudACPLoginSession): ACPAuthLogin | undefined {
  if (!session) return undefined
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
