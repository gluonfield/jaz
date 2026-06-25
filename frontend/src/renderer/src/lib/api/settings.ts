import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, post, put } from './client'
import type { ACPAgentAuth, ACPAuthLogin, AgentSettings, BrowserMode, BrowserStatus } from './types'

function normalizeAgentSettings(settings: AgentSettings): AgentSettings {
  return {
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
            model_provider: current.model_provider?.trim() || '',
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

// The exact shape a save writes back: editable fields only, normalized. Also the
// canonical basis for change detection — settings unchanged here means a no-op save.
export function inputFromSettings(
  settings: AgentSettings,
  providerKeys?: Record<string, string>,
): AgentSettings & { provider_keys?: Record<string, string> } {
  const normalized = normalizeAgentSettings(settings)
  const keys = compactKeys(providerKeys)
  return {
    providers: normalized.providers,
    acp: normalized.acp,
    acp_keys: normalized.acp_keys,
    agents: normalized.agents,
    ...(keys ? { provider_keys: keys } : {}),
  }
}

// Trims a secret map and drops blanks; undefined when nothing remains. Used for
// both model provider keys and ACP agent keys before they hit the backend.
export function compactKeys(values?: Record<string, string>): Record<string, string> | undefined {
  if (!values) return undefined
  const out = Object.fromEntries(
    Object.entries(values)
      .map(([key, value]) => [key, value.trim()] as const)
      .filter(([, value]) => value.length > 0),
  )
  return Object.keys(out).length > 0 ? out : undefined
}

// A deep-ish clone of the editable agent settings, so a draft can be mutated
// without touching the cached query data. The canonical copy — both the
// onboarding and settings screens edit drafts of this shape.
export function cloneAgentSettings(settings: AgentSettings): AgentSettings {
  return {
    providers: [...(settings.providers ?? [])],
    acp_auth: { ...(settings.acp_auth ?? {}) },
    acp_keys: { ...(settings.acp_keys ?? {}) },
    acp: Object.fromEntries(
      Object.entries(settings.acp ?? {}).map(([agent, value]) => [
        agent,
        { ...value, auth: value.auth ? { ...value.auth } : undefined },
      ]),
    ),
    agents: [...(settings.agents ?? [])],
    acp_options: Object.fromEntries(
      Object.entries(settings.acp_options ?? {}).map(([agent, value]) => [
        agent,
        {
          ...value,
          capabilities: value.capabilities ? { ...value.capabilities } : undefined,
          reasoning_efforts: [...value.reasoning_efforts],
          model_provider_ids: [...(value.model_provider_ids ?? [])],
        },
      ]),
    ),
  }
}

export const agentSettingsQuery = queryOptions({
  queryKey: keys.agentSettings,
  queryFn: async () => normalizeAgentSettings(await get<AgentSettings>('/v1/settings/agents')),
})

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

export function updateBrowserSettings(input: {
  enabled?: boolean
  agent?: string
  mode?: BrowserMode
}): Promise<BrowserStatus> {
  return put<BrowserStatus>('/v1/browser', input)
}

export function startACPAuthLogin(agent: string, auth?: ACPAgentAuth): Promise<ACPAuthLogin> {
  return post<ACPAuthLogin>(`/v1/acp/agents/${encodeURIComponent(agent)}/auth/login`, { auth })
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
