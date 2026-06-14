import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, post, put } from './client'
import type { ACPAgentAuth, ACPAuthLogin, AgentSettings } from './types'

function normalizeAgentSettings(settings: AgentSettings): AgentSettings {
  return {
    native: {
      model_provider: settings.native.model_provider?.trim() || undefined,
      model: settings.native.model.trim(),
      reasoning_effort: settings.native.reasoning_effort ?? '',
    },
    providers: settings.providers ?? [],
    acp_auth: settings.acp_auth ?? {},
    acp_keys: settings.acp_keys ?? {},
    acp_options: settings.acp_options ?? {},
    acp: Object.fromEntries(
      (settings.agents ?? []).map((agent) => {
        const current = settings.acp?.[agent] ?? { enabled: false }
        return [
          agent,
          {
            enabled: Boolean(current.enabled),
            command: current.command?.trim() || '',
            model: current.model?.trim() || '',
            reasoning_effort: current.reasoning_effort ?? '',
            auth: {
              mode: current.auth?.mode || 'auto',
              path: current.auth?.path?.trim() || '',
            },
          },
        ]
      }),
    ),
    agents: settings.agents ?? [],
  }
}

function inputFromSettings(
  settings: AgentSettings,
  providerKeys?: Record<string, string>,
): AgentSettings & { provider_keys?: Record<string, string> } {
  const normalized = normalizeAgentSettings(settings)
  const keys = compactKeys(providerKeys)
  return {
    native: normalized.native,
    providers: normalized.providers,
    acp: normalized.acp,
    acp_keys: normalized.acp_keys,
    agents: normalized.agents,
    ...(keys ? { provider_keys: keys } : {}),
  }
}

function compactKeys(values?: Record<string, string>): Record<string, string> | undefined {
  if (!values) return undefined
  const out = Object.fromEntries(
    Object.entries(values)
      .map(([key, value]) => [key, value.trim()] as const)
      .filter(([, value]) => value.length > 0),
  )
  return Object.keys(out).length > 0 ? out : undefined
}

export const agentSettingsQuery = queryOptions({
  queryKey: keys.agentSettings,
  queryFn: async () => normalizeAgentSettings(await get<AgentSettings>('/v1/settings/agents')),
})

// providerKeys maps a native provider id (e.g. "openrouter") to a freshly
// pasted API key; the backend stores it as that provider's key env var.
export function updateAgentSettings(
  settings: AgentSettings,
  providerKeys?: Record<string, string>,
): Promise<AgentSettings> {
  return put<AgentSettings>('/v1/settings/agents', inputFromSettings(settings, providerKeys)).then(
    normalizeAgentSettings,
  )
}

export function startACPAuthLogin(agent: string, auth?: ACPAgentAuth): Promise<ACPAuthLogin> {
  return post<ACPAuthLogin>(`/v1/acp/agents/${encodeURIComponent(agent)}/auth/login`, { auth })
}

export function getACPAuthLogin(id: string): Promise<ACPAuthLogin> {
  return get<ACPAuthLogin>(`/v1/acp/auth-logins/${encodeURIComponent(id)}`)
}
