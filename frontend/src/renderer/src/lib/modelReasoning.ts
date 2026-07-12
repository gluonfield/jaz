import { useQuery } from '@tanstack/react-query'
import type { AgentSettings, ModelProviderOption, ReasoningEffortOption } from './api/types'
import {
  acpAgentModelSuggestions,
  modelProviderModelsQuery,
  modelSuggestionsForProvider,
  type ModelSuggestion,
} from './models'
import {
  modelReasoningEffortOptions,
  modelSettingsReasoningEffortOptions,
} from './reasoningEfforts'

export interface ModelReasoningState {
  modelSuggestions: ModelSuggestion[]
  modelsLoading: boolean
  reasoningOptions: ReasoningEffortOption[]
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
  const reasoningOptions = settingsMode
    ? modelSettingsReasoningEffortOptions(settings, agent, model, modelSuggestions, reasoningEffort)
    : modelReasoningEffortOptions(settings, agent, model, modelSuggestions, reasoningEffort)
  return {
    modelSuggestions,
    modelsLoading: providerModels.isLoading,
    reasoningOptions,
  }
}
