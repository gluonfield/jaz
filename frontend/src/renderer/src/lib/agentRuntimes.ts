import type { AgentSettings, NativeProviderOption } from './api/types'

export function enabledACPAgents(settings?: AgentSettings): string[] {
  return (settings?.agents ?? []).filter((agent) => settings?.acp[agent]?.enabled)
}

export function configuredNativeProviders(settings?: AgentSettings): NativeProviderOption[] {
  return (settings?.providers ?? []).filter((provider) => provider.implemented && provider.configured)
}

export function effectiveNativeProvider(settings?: AgentSettings, requested?: string | null): string {
  const providers = configuredNativeProviders(settings)
  const ids = new Set(providers.map((provider) => provider.id))
  const preferred = requested || settings?.native.model_provider || ''
  if (ids.has(preferred)) return preferred
  return providers[0]?.id ?? ''
}
