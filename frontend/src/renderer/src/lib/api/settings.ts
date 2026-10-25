import { queryOptions } from '@tanstack/react-query'
import { inputFromSettings, normalizeAgentSettings } from '@/lib/agentSettings'
import { keys } from '../query/keys'
import { get, post, put } from './client'
import type { ACPAgentAuth, ACPAuthLogin, AgentSettings, BrowserStatus } from './types'

export const agentSettingsQuery = queryOptions({
  queryKey: keys.agentSettings,
  queryFn: async () => normalizeAgentSettings(await get<AgentSettings>('/v1/settings/agents')),
})

export const computerSettingsQuery = queryOptions({
  queryKey: keys.computerSettings,
  queryFn: () => get<{ enabled: boolean }>('/v1/computer'),
})

export function updateComputerSettings(enabled: boolean): Promise<{ enabled: boolean }> {
  return put('/v1/computer', { enabled })
}

export const browserSettingsQuery = queryOptions({
  queryKey: keys.browserSettings,
  queryFn: () => get<BrowserStatus>('/v1/browser'),
  refetchInterval: 2000,
})

// providerKeys maps a model provider id (e.g. "openrouter") to a freshly
// pasted API key; the backend stores it as that provider's key env var.
export function updateAgentSettings(
  settings: AgentSettings,
  providerKeys?: Record<string, string>,
): Promise<AgentSettings> {
  return put<AgentSettings>('/v1/settings/agents', inputFromSettings(settings, providerKeys)).then(
    normalizeAgentSettings,
  )
}

export function updateBrowserSettings(enabled: boolean): Promise<BrowserStatus> {
  return put<BrowserStatus>('/v1/browser', { enabled })
}

export function startACPAuthLogin(agent: string, auth?: ACPAgentAuth): Promise<ACPAuthLogin> {
  return post<ACPAuthLogin>(`/v1/acp/agents/${encodeURIComponent(agent)}/auth/login`, { auth })
}

export function prepareACPAgent(agent: string): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/acp/agents/${encodeURIComponent(agent)}/prepare`)
}

export function getACPAuthLogin(id: string): Promise<ACPAuthLogin> {
  return get<ACPAuthLogin>(`/v1/acp/auth-logins/${encodeURIComponent(id)}`)
}

// Hands a code the browser printed back to a login process that's blocked
// waiting for it — the remote/headless flow, where the CLI can't capture an
// OAuth redirect on the user's machine.
export function submitACPAuthLoginInput(id: string, input: string): Promise<ACPAuthLogin> {
  return post<ACPAuthLogin>(`/v1/acp/auth-logins/${encodeURIComponent(id)}/input`, { input })
}

// Removes an agent's Jaz-managed credential (API key env + Jaz-profile OAuth);
// never touches the user's global CLI config. Returns the canonical settings snapshot.
export function disconnectACPAuth(agent: string): Promise<AgentSettings> {
  return post<AgentSettings>(`/v1/acp/agents/${encodeURIComponent(agent)}/auth/disconnect`).then(
    normalizeAgentSettings,
  )
}
