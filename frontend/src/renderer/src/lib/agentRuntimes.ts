import type { AgentSettings, ModelProviderOption } from './api/types'

export const ACP_PROVIDER_MODE_AGENT = 'agent_defaults'
// Providers without first-class support yet — kept in the catalog for the runtime
// but hidden from selection and the settings providers list. Remove an id here
// once the provider is ready to surface in the UI.
const HIDDEN_PROVIDERS = new Set(['ollama'])
const AUTH_REQUIRED_AGENTS = new Set(['codex', 'claude', 'grok', 'opencode'])

export function providerHidden(id: string): boolean {
  return HIDDEN_PROVIDERS.has(id)
}

export function enabledACPAgents(settings?: AgentSettings): string[] {
  return (settings?.agents ?? []).filter((agent) => acpAgentEnabled(settings, agent))
}

export function acpAgentSupportsNativeGoal(settings: AgentSettings | undefined, agent: string): boolean {
  return settings?.acp_options?.[agent]?.capabilities?.native_goal === true
}

export function selectableACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption[] {
  if (!acpUsesModelProvider(settings, agent)) return []
  const ids = new Set(settings?.acp_options?.[agent]?.model_provider_ids ?? [])
  return (settings?.providers ?? []).filter(
    (provider) => ids.has(provider.id) && !providerHidden(provider.id),
  )
}

// The backend serializes `requires_api_key` with omitempty, so the only values
// that reach us are `true` (needs a key) or absent (no key needed) — never an
// explicit `false`. These are the single source of truth for "does this provider
// need a key" and "is it ready to use"; reuse them everywhere instead of
// re-deriving the predicate (the variants drift apart on the absent case).
export function modelProviderRequiresKey(provider: ModelProviderOption): boolean {
  return provider.requires_api_key === true
}

export function modelProviderConnected(provider: ModelProviderOption): boolean {
  return Boolean(provider.configured) || !modelProviderRequiresKey(provider)
}

export function configuredACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption[] {
  return selectableACPModelProviders(settings, agent).filter(modelProviderConnected)
}

export function selectedACPModelProvider(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption | undefined {
  const selected = settings?.acp[agent]?.model_provider ?? ''
  return selectableACPModelProviders(settings, agent).find((provider) => provider.id === selected)
}

export function acpAgentEnableable(settings: AgentSettings | undefined, agent: string): boolean {
  if (!acpAgentAuthReady(settings, agent)) return false
  if (!acpUsesModelProvider(settings, agent)) return true
  const provider = selectedACPModelProvider(settings, agent)
  return Boolean(provider && modelProviderConnected(provider))
}

function acpAgentAuthReady(settings: AgentSettings | undefined, agent: string): boolean {
  const options = settings?.acp_options?.[agent]
  if (options?.local || !AUTH_REQUIRED_AGENTS.has(agent)) return true
  if (settings?.acp_auth?.[agent]?.authenticated) return true
  return Boolean(settings?.acp_keys?.[agent]?.trim())
}

export function acpAgentEnabled(settings: AgentSettings | undefined, agent: string): boolean {
  return Boolean(settings?.acp[agent]?.enabled) && acpAgentEnableable(settings, agent)
}

export function normalizeACPAgentEnabled(settings: AgentSettings, agent: string): AgentSettings {
  const current = settings.acp[agent]
  if (!current?.enabled || acpAgentEnableable(settings, agent)) return settings
  return {
    ...settings,
    acp: {
      ...settings.acp,
      [agent]: { ...current, enabled: false },
    },
  }
}

export function normalizeACPAgentsEnabled(settings: AgentSettings): AgentSettings {
  return (settings.agents ?? []).reduce(
    (next, agent) => normalizeACPAgentEnabled(next, agent),
    settings,
  )
}

export function effectiveACPModelProvider(
  settings: AgentSettings | undefined,
  agent: string,
  requested?: string | null,
): string {
  const providers = configuredACPModelProviders(settings, agent)
  const ids = new Set(providers.map((provider) => provider.id))
  const preferred = requested || settings?.acp[agent]?.model_provider || ''
  if (ids.has(preferred)) return preferred
  return providers[0]?.id ?? ''
}

export function runtimeModelState(
  settings: AgentSettings | undefined,
  runtime: string,
  requestedProvider?: string | null,
) {
  const usesModelProvider = acpUsesModelProvider(settings, runtime)
  const usesProvider = usesModelProvider
  const providers = usesModelProvider ? configuredACPModelProviders(settings, runtime) : []
  const defaultProvider = settings?.acp[runtime]?.model_provider ?? ''
  const provider = usesModelProvider
    ? effectiveACPModelProvider(settings, runtime, requestedProvider)
    : ''
  const selectedProvider = providers.find((p) => p.id === provider)
  const defaultModel = usesModelProvider
    ? provider === defaultProvider
      ? (settings?.acp[runtime]?.model ?? '')
      : (selectedProvider?.default_model ?? '')
    : (settings?.acp[runtime]?.model ?? '')
  const defaultEffort = settings?.acp[runtime]?.reasoning_effort ?? ''
  return {
    usesModelProvider,
    usesProvider,
    providers,
    provider,
    selectedProvider,
    defaultModel,
    defaultEffort,
  }
}

export function acpUsesModelProvider(settings: AgentSettings | undefined, agent: string): boolean {
  return settings?.acp_options?.[agent]?.provider_mode === ACP_PROVIDER_MODE_AGENT
}

export function acpAgentRunnable(settings: AgentSettings | undefined, agent: string): boolean {
  return acpAgentEnableable(settings, agent)
}

// Where to grab an API key for each backend provider id.
const PROVIDER_KEY_URLS: Record<string, string> = {
  openrouter: 'https://openrouter.ai/keys',
  openai: 'https://platform.openai.com/api-keys',
}

export function providerKeyUrl(providerId: string): string | undefined {
  return PROVIDER_KEY_URLS[providerId]
}
