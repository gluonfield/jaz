import { useQuery } from '@tanstack/react-query'
import type { AgentSettings, ModelProviderOption, ReasoningEffortOption } from './api/types'
import {
  acpAgentModelSuggestions,
  modelProviderModelsQuery,
  modelSuggestionsForProvider,
  type ModelSuggestion,
} from './models'
import {
  modelReasoningSelection,
  type ModelReasoningCatalog,
  type ModelReasoningSelection,
} from './reasoningEfforts'

export interface ModelReasoningState {
  modelSuggestions: ModelSuggestion[]
  modelsLoading: boolean
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
}: {
  settings: AgentSettings | undefined
  agent: string
  model: string
  reasoningEffort: string
  usesProvider: boolean
  provider: string | undefined
  selectedProvider: ModelProviderOption | undefined
  settingsMode?: boolean
}): ModelReasoningState {
  const providerModels = useQuery({
    ...modelProviderModelsQuery(provider, agent),
    enabled: usesProvider && Boolean(provider),
  })
  const modelSuggestions = usesProvider
    ? modelSuggestionsForProvider(selectedProvider, providerModels.data ?? [])
    : acpAgentModelSuggestions(settings, agent)
  const catalog: ModelReasoningCatalog = !usesProvider
    ? { status: 'ready', suggestions: modelSuggestions, unknownModel: 'ready' }
    : providerModels.data !== undefined
      ? { status: 'ready', suggestions: modelSuggestions, unknownModel: 'unavailable' }
      : providerModels.isError
        ? { status: 'error' }
        : { status: 'pending' }
  const reasoningForModel = (nextModel: string, effort: string) => modelReasoningSelection({
    settings,
    agent,
    model: nextModel,
    requested: effort,
    settingsMode,
    catalog,
  })
  const reasoning = reasoningForModel(model, reasoningEffort)
  return {
    modelSuggestions,
    modelsLoading: providerModels.isLoading,
    reasoningOptions: reasoning.options,
    effectiveReasoningEffort: reasoning.effectiveEffort,
    reasoningEffortSupported: reasoning.supported,
    reasoningStatus: reasoning.status,
    reasoningBlocked: reasoning.blocked,
    reasoningForModel,
  }
}
