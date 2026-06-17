import type { AgentSettings, NativeProviderOption } from './api/types'

export const ACP_PROVIDER_MODE_NATIVE = 'native_defaults'
export const ACP_PROVIDER_MODE_AGENT = 'agent_defaults'
const HIDDEN_MODEL_PROVIDERS = new Set(['ollama'])

export function enabledACPAgents(settings?: AgentSettings): string[] {
  return (settings?.agents ?? []).filter(
    (agent) => settings?.acp[agent]?.enabled && acpAgentRunnable(settings, agent),
  )
}

export function configuredNativeProviders(settings?: AgentSettings): NativeProviderOption[] {
  return (settings?.providers ?? []).filter((provider) => provider.implemented && provider.configured)
}

export function selectableACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): NativeProviderOption[] {
  if (!acpUsesModelProvider(settings, agent)) return []
  const ids = new Set(settings?.acp_options?.[agent]?.model_provider_ids ?? [])
  return (settings?.providers ?? []).filter(
    (provider) => ids.has(provider.id) && !HIDDEN_MODEL_PROVIDERS.has(provider.id),
  )
}

export function configuredACPModelProviders(
  settings: AgentSettings | undefined,
  agent: string,
): NativeProviderOption[] {
  return selectableACPModelProviders(settings, agent).filter(
    (provider) => provider.configured || !provider.requires_api_key,
  )
}

export function effectiveNativeProvider(settings?: AgentSettings, requested?: string | null): string {
  const providers = configuredNativeProviders(settings)
  const ids = new Set(providers.map((provider) => provider.id))
  const preferred = requested || settings?.native.model_provider || ''
  if (ids.has(preferred)) return preferred
  return providers[0]?.id ?? ''
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
  const native = runtime === 'native'
  const usesNativeProvider = native || acpUsesNativeProvider(settings, runtime)
  const usesModelProvider = !native && acpUsesModelProvider(settings, runtime)
  const usesProvider = usesNativeProvider || usesModelProvider
  const providers = usesNativeProvider
    ? configuredNativeProviders(settings)
    : usesModelProvider
      ? configuredACPModelProviders(settings, runtime)
      : []
  const defaultProvider = usesNativeProvider
    ? (settings?.native.model_provider ?? '')
    : (settings?.acp[runtime]?.model_provider ?? '')
  const provider = usesNativeProvider
    ? effectiveNativeProvider(settings, requestedProvider)
    : usesModelProvider
      ? effectiveACPModelProvider(settings, runtime, requestedProvider)
      : ''
  const selectedProvider = providers.find((p) => p.id === provider)
  const defaultModel = usesNativeProvider
    ? provider === defaultProvider
      ? (settings?.native.model ?? '')
      : (selectedProvider?.default_model ?? '')
    : usesModelProvider
      ? provider === defaultProvider
        ? (settings?.acp[runtime]?.model ?? '')
        : (selectedProvider?.default_model ?? '')
      : (settings?.acp[runtime]?.model ?? '')
  const defaultEffort = usesNativeProvider
    ? (settings?.native.reasoning_effort ?? '')
    : (settings?.acp[runtime]?.reasoning_effort ?? '')
  return {
    usesNativeProvider,
    usesModelProvider,
    usesProvider,
    providers,
    provider,
    selectedProvider,
    defaultModel,
    defaultEffort,
  }
}

export function acpUsesNativeProvider(settings: AgentSettings | undefined, agent: string): boolean {
  return settings?.acp_options?.[agent]?.provider_mode === ACP_PROVIDER_MODE_NATIVE
}

export function acpUsesModelProvider(settings: AgentSettings | undefined, agent: string): boolean {
  return settings?.acp_options?.[agent]?.provider_mode === ACP_PROVIDER_MODE_AGENT
}

export function acpAgentRunnable(settings: AgentSettings | undefined, agent: string): boolean {
  if (acpUsesNativeProvider(settings, agent)) return configuredNativeProviders(settings).length > 0
  if (acpUsesModelProvider(settings, agent)) return configuredACPModelProviders(settings, agent).length > 0
  return true
}
