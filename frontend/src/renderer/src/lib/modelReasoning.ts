import { useQuery } from '@tanstack/react-query'
import type { AgentSettings, ModelProviderOption, ReasoningEffortOption } from './api/types'
import {
  acpAgentModelSuggestions,
  modelProviderModelsQuery,
  modelSuggestionsForProvider,
  type ModelSuggestion,
} from './models'
import { modelReasoningSelection } from './reasoningEfforts'

export interface ModelReasoningState {
  modelSuggestions: ModelSuggestion[]
  modelsLoading: boolean
  reasoningOptions: ReasoningEffortOption[]
  effectiveReasoningEffort: string
  reasoningEffortSupported: boolean
  reasoningPending: boolean
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
  const capabilitiesPending = usesProvider && providerModels.data === undefined && providerModels.isFetching
  const reasoning = modelReasoningSelection(
    settings,
    agent,
    model,
    capabilitiesPending ? [] : modelSuggestions,
    reasoningEffort,
    settingsMode,
    capabilitiesPending ? 'pending' : usesProvider ? 'unavailable' : 'ready',
  )
  return {
    modelSuggestions,
    modelsLoading: providerModels.isLoading,
    reasoningOptions: reasoning.options,
    effectiveReasoningEffort: reasoning.effectiveEffort,
    reasoningEffortSupported: reasoning.supported,
    reasoningPending: reasoning.pending,
  }
}
