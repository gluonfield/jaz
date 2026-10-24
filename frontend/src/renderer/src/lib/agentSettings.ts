import { selectableACPAgent } from './agentRuntimes'
import type { AgentSettings, AgentSettingsInput } from './api/types'

export function normalizeAgentSettings(settings: AgentSettings): AgentSettings {
  const agents = (settings.agents ?? []).filter(selectableACPAgent)
  return {
    providers: settings.providers ?? [],
    acp_auth: settings.acp_auth ?? {},
    acp_options: cloneACPOptions(settings.acp_options),
    ...inputFromSettings(settings),
    agents,
  }
}

function cloneACPOptions(options: AgentSettings['acp_options']): AgentSettings['acp_options'] {
  return Object.fromEntries(
    Object.entries(options ?? {}).map(([agent, value]) => [
      agent,
      {
        ...value,
        reasoning_efforts: [...(value.reasoning_efforts ?? [])],
        models: value.models?.map((model) => ({
          ...model,
          aliases: model.aliases ? [...model.aliases] : undefined,
          reasoning: {
            ...model.reasoning,
            efforts: model.reasoning.efforts ? [...model.reasoning.efforts] : undefined,
          },
          pricing: model.pricing ? { ...model.pricing } : undefined,
        })),
        model_provider_ids: [...(value.model_provider_ids ?? [])],
        model_providers: [...(value.model_providers ?? [])],
      },
    ]),
  )
}

// The exact shape a save writes back: editable fields only, normalized. Also the
// canonical basis for change detection — settings unchanged here means a no-op save.
export function inputFromSettings(
  settings: AgentSettings,
  providerKeys?: Record<string, string>,
): AgentSettingsInput & { provider_keys?: Record<string, string> } {
  const agents = (settings.agents ?? []).filter(selectableACPAgent)
  const keys = compactKeys(providerKeys)
  return {
    acp: Object.fromEntries(
      agents.map((agent) => {
        const current = settings.acp?.[agent] ?? { enabled: false }
        return [
          agent,
          {
            enabled: Boolean(current.enabled),
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
    acp_keys: { ...(settings.acp_keys ?? {}) },
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
    acp_options: cloneACPOptions(settings.acp_options),
  }
}

