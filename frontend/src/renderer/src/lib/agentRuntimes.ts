import { agentDefault } from './jazDefaults'
import type { AgentSettings, ModelProviderOption } from './api/types'

export const ACP_PROVIDER_MODE_AGENT = 'agent_defaults'
// Providers without first-class support yet — kept in the catalog for the runtime
// but hidden from selection and the settings providers list. Remove an id here
// once the provider is ready to surface in the UI.
const HIDDEN_PROVIDERS = new Set(['ollama'])
const HIDDEN_AGENTS = new Set(['jaz'])
const AUTH_REQUIRED_AGENTS = new Set(['codex', 'claude', 'grok', 'opencode', 'antigravity'])

export function providerHidden(id: string): boolean {
  return HIDDEN_PROVIDERS.has(id)
}

export function enabledACPAgents(settings?: AgentSettings): string[] {
  return (settings?.agents ?? []).filter((agent) => selectableACPAgent(agent) && acpAgentEnabled(settings, agent))
}

export function selectableACPAgent(agent: string | undefined): boolean {
  const slug = (agent ?? '').trim().toLowerCase()
  return Boolean(slug) && !HIDDEN_AGENTS.has(slug)
}

export function acpAgentSupportsGoal(agent: string): boolean {
  return selectableACPAgent(agent)
}

export function selectableACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption[] {
  if (!acpUsesModelProvider(settings, agent)) return []
  const configuredOptions = settings?.acp_options?.[agent]?.model_providers
  if (configuredOptions) {
    return configuredOptions.filter((provider) => !providerHidden(provider.id))
  }
  const ids = settings?.acp_options?.[agent]?.model_provider_ids ?? []
  const byID = new Map((settings?.providers ?? []).map((provider) => [provider.id, provider]))
  const providers = ids
    .map((id) => byID.get(id))
    .filter((provider): provider is ModelProviderOption => Boolean(provider && !providerHidden(provider.id)))
  return orderedACPModelProviders(settings, agent, providers)
}

function orderedACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
  providers: ModelProviderOption[],
): ModelProviderOption[] {
  const first = settings?.acp_options?.[agent]?.auth_provider_id
  if (!first) return providers
  return [...providers].sort((a, b) => Number(b.id === first) - Number(a.id === first))
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
  if (provider.connection_status) return provider.connection_status === 'connected'
  return Boolean(provider.configured) || !modelProviderRequiresKey(provider)
}

export function configuredACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption[] {
  return selectableACPModelProviders(settings, agent).filter((provider) =>
    acpModelProviderReady(settings, agent, provider),
  )
}

export function acpProviderUsesNativeAuth(
  settings: AgentSettings | undefined,
  agent: string,
  providerId: string | undefined,
): boolean {
  const options = settings?.acp_options?.[agent]
  return Boolean(
    providerId && options?.supports_auth && options.auth_provider_id === providerId,
  )
}

export function acpModelProviderReady(
  settings: AgentSettings | undefined,
  agent: string,
  provider: ModelProviderOption,
): boolean {
  if (acpProviderUsesNativeAuth(settings, agent, provider.id)) {
    return acpAgentAuthReady(settings, agent)
  }
  return modelProviderConnected(provider)
}

export function selectedACPModelProvider(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption | undefined {
  const selected = settings?.acp[agent]?.model_provider ?? ''
  return selectableACPModelProviders(settings, agent).find((provider) => provider.id === selected)
}

export function acpAgentEnableable(settings: AgentSettings | undefined, agent: string): boolean {
  if (!acpUsesModelProvider(settings, agent)) return acpAgentAuthReady(settings, agent)
  const provider = selectedACPModelProvider(settings, agent)
  return Boolean(provider && acpModelProviderReady(settings, agent, provider))
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
  const onDefaultProvider = !usesModelProvider || provider === defaultProvider
  // Deploy defaults (jaz-defaults.js) seed a fresh thread, then the user's pick
  // wins. Model applies only on the agent's own provider; another provider keeps
  // its own default model.
  const deploy = agentDefault(runtime)
  const defaultModel = onDefaultProvider
    ? deploy.model?.trim() || settings?.acp[runtime]?.model || ''
    : (selectedProvider?.default_model ?? '')
  const defaultEffort = deploy.reasoningEffort?.trim() || settings?.acp[runtime]?.reasoning_effort || ''
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
