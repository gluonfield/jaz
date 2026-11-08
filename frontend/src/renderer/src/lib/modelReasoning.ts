import { useQuery } from '@tanstack/react-query'
import type { AgentSessionConfigOption, AgentSettings, ModelProviderOption, ReasoningEffortOption } from '@/lib/api/types'
import {
  acpAgentModelSuggestions,
  agentModelsQuery,
  modelProviderModelsQuery,
  modelSuggestionsForProvider,
  type ModelSuggestion,
} from '@/lib/models'
import {
  modelReasoningSelection,
  sessionReasoningEffortOptions,
  type ModelReasoningCatalog,
  type ModelReasoningSelection,
} from '@/lib/reasoningEfforts'

export interface ModelReasoningState {
  modelSuggestions: ModelSuggestion[]
  modelsLoading: boolean
  defaultModel?: string
  modelsError?: string
  reasoningOptions: ReasoningEffortOption[]
  effectiveReasoningEffort: string
  reasoningEffortSupported: boolean
  reasoningStatus: ModelReasoningSelection['status']
  reasoningBlocked: boolean
  reasoningForModel: (model: string, effort: string) => ModelReasoningSelection
}

export function useModelReasoningState({
  settings,
  agent,
  model,
  reasoningEffort,
  usesProvider,
  provider,
  selectedProvider,
  settingsMode = false,
  sessionOptions,
}: {
  settings: AgentSettings | undefined
  agent: string
  model: string
  reasoningEffort: string
  usesProvider: boolean
  provider: string | undefined
  selectedProvider: ModelProviderOption | undefined
  settingsMode?: boolean
  sessionOptions?: AgentSessionConfigOption[] | null
}): ModelReasoningState {
  const providerModels = useQuery({
    ...modelProviderModelsQuery(provider, agent),
    enabled: usesProvider && Boolean(provider),
  })
  const nativeEnabled = !usesProvider && settings?.acp_auth?.[agent]?.ready === true
  const nativeModels = useQuery({ ...agentModelsQuery(agent), enabled: nativeEnabled })
  const modelSuggestions = usesProvider
    ? modelSuggestionsForProvider(selectedProvider, providerModels.data ?? [])
    : nativeModels.data?.models ?? acpAgentModelSuggestions(settings, agent)
  const query = usesProvider ? providerModels : nativeModels
  const ready = usesProvider
    ? providerModels.data !== undefined
    : !nativeEnabled || nativeModels.data !== undefined
  const defaultModel = usesProvider ? undefined : nativeModels.data?.defaultModel
  const catalog: ModelReasoningCatalog = ready
    ? { status: 'ready', suggestions: modelSuggestions, unknownModel: usesProvider ? 'unavailable' : 'ready' }
    : { status: query.isError ? 'error' : 'pending' }
  const reasoningForModel = (nextModel: string, effort: string) => modelReasoningSelection({
    settings,
    agent,
    model: nextModel || defaultModel || '',
    requested: effort,
    settingsMode,
    catalog,
    advertisedOptions: nextModel === model ? sessionReasoningEffortOptions(sessionOptions) : undefined,
  })
  const reasoning = reasoningForModel(model, reasoningEffort)
  return {
    modelSuggestions,
    modelsLoading: query.isLoading,
    defaultModel,
    modelsError: query.error?.message || (nativeEnabled && nativeModels.data?.models.length === 0
      ? 'No models reported by this agent.'
      : undefined),
    reasoningOptions: reasoning.options,
    effectiveReasoningEffort: reasoning.effectiveEffort,
    reasoningEffortSupported: reasoning.supported,
    reasoningStatus: reasoning.status,
    reasoningBlocked: reasoning.blocked,
    reasoningForModel,
  }
}
