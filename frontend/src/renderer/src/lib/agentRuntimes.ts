import type { AgentSettings, ModelProviderOption } from './api/types'

export const ACP_PROVIDER_MODE_AGENT = 'agent_defaults'
const HIDDEN_MODEL_PROVIDERS = new Set(['ollama'])

export function enabledACPAgents(settings?: AgentSettings): string[] {
  return (settings?.agents ?? []).filter(
    (agent) => settings?.acp[agent]?.enabled && acpAgentRunnable(settings, agent),
  )
}

export function selectableACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption[] {
  if (!acpUsesModelProvider(settings, agent)) return []
  const ids = new Set(settings?.acp_options?.[agent]?.model_provider_ids ?? [])
  return (settings?.providers ?? []).filter(
    (provider) => ids.has(provider.id) && !HIDDEN_MODEL_PROVIDERS.has(provider.id),
  )
}

export function configuredACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): ModelProviderOption[] {
  return selectableACPModelProviders(settings, agent).filter(
    (provider) => provider.configured || !provider.requires_api_key,
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
  if (acpUsesModelProvider(settings, agent)) return configuredACPModelProviders(settings, agent).length > 0
  return true
}

// Where to grab an API key for each backend provider id.
const PROVIDER_KEY_URLS: Record<string, string> = {
  openrouter: 'https://openrouter.ai/keys',
  openai: 'https://platform.openai.com/api-keys',
}

export function providerKeyUrl(providerId: string): string | undefined {
  return PROVIDER_KEY_URLS[providerId]
}
