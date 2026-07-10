import { queryOptions, useQuery } from '@tanstack/react-query'
import { get } from './api/client'
import { agentSettingsQuery } from './api/settings'
import type { AgentSettings, ModelCatalogEntry, ModelProviderOption, Session } from './api/types'
import { keys } from './query/keys'

// USD per token, parsed from OpenRouter's string pricing fields.
export interface ModelPricing {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
}

export interface ModelSuggestion {
  value: string
  label: string
  description?: string
  contextLength?: number
  pricing?: ModelPricing
  openRouterId?: string
  reasoningEfforts?: string[]
  reasoningDefaultEffort?: string
  reasoningMandatory?: boolean
}

export function acpAgentModelSuggestions(
  settings: AgentSettings | undefined,
  agent: string,
): ModelSuggestion[] {
  return modelSuggestionsFromCatalog(settings?.acp_options?.[agent]?.models ?? [])
}

// '' resolves to the first suggestion — the model an unset value runs as.
export function modelSuggestionFor(
  suggestions: ModelSuggestion[],
  value: string,
): ModelSuggestion | undefined {
  const raw = value.trim()
  return raw ? suggestions.find((s) => s.value === raw) : suggestions[0]
}

function usageCatalogEntry(
  model: { agent?: string; model?: string },
  settings?: AgentSettings,
): ModelSuggestion | undefined {
  if (!settings || !model.agent) return undefined
  return modelSuggestionFor(acpAgentModelSuggestions(settings, model.agent), model.model ?? '')
}

export function pricingIdForUsage(
  model: { agent?: string; model?: string },
  settings?: AgentSettings,
): string | null {
  const openRouterId = usageCatalogEntry(model, settings)?.openRouterId
  if (openRouterId) return openRouterId
  const raw = model.model?.trim() ?? ''
  if (!raw) return null
  const direct = raw.replace(/^openrouter\//, '')
  return direct.includes('/') ? direct : null
}

export function usageModelLabel(
  model: { agent?: string; model?: string },
  settings?: AgentSettings,
): string {
  return usageCatalogEntry(model, settings)?.label || model.model?.trim() || 'Unknown model'
}

export function modelSuggestionsForProvider(
  provider: ModelProviderOption | undefined,
  providerModels: ModelSuggestion[] = [],
): ModelSuggestion[] {
  if (!provider) return []
  if (providerModels.length > 0) return providerModels
  if (provider.default_model) {
    return [{ value: provider.default_model, label: provider.default_model, description: provider.label }]
  }
  return []
}

export function modelProviderModelsQuery(provider: string | undefined, agent = '') {
  const id = provider ?? ''
  return queryOptions({
    queryKey: keys.modelProviderModels(id, agent),
    queryFn: async (): Promise<ModelSuggestion[]> => {
      const query = agent ? `?agent=${encodeURIComponent(agent)}` : ''
      const body = await get<{ models?: ModelCatalogEntry[] }>(
        `/v1/model-providers/${encodeURIComponent(id)}/models${query}`,
      )
      return modelSuggestionsFromCatalog(body.models ?? [])
    },
    staleTime: 60 * 60 * 1000,
    retry: 1,
  })
}

export const openRouterModelsQuery = modelProviderModelsQuery('openrouter')

function modelSuggestionsFromCatalog(models: ModelCatalogEntry[]): ModelSuggestion[] {
  return models.map((model) => ({
    value: model.value,
    label: model.label,
    description: model.description,
    contextLength: model.context_length || undefined,
    pricing: model.pricing
      ? {
          input: model.pricing.input,
          output: model.pricing.output,
          cacheRead: model.pricing.cache_read,
          cacheWrite: model.pricing.cache_write,
        }
      : undefined,
    openRouterId: model.openrouter_id,
    reasoningEfforts: Array.isArray(model.reasoning_efforts) ? model.reasoning_efforts : undefined,
    reasoningDefaultEffort: model.reasoning_default_effort,
    reasoningMandatory: model.reasoning_mandatory,
  }))
}

export function filterModelSuggestions(
  suggestions: ModelSuggestion[],
  query: string,
): ModelSuggestion[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return suggestions
  return suggestions.filter(
    (s) => s.value.toLowerCase().includes(needle) || s.label.toLowerCase().includes(needle),
  )
}

export function modelSuggestionLabel(suggestions: ModelSuggestion[], value: string): string {
  return suggestions.find((s) => s.value === value)?.label ?? value
}

// Context window for a session, by precedence: the runtime-reported size
// (ACP usage_update), then a known model entry (curated or OpenRouter's
// catalog), then the model-family heuristic. Null hides the capacity readout.
export function useContextWindow(session: Session): number | null {
  const usage = session.usage
  const hasTokens = Boolean(
    usage &&
      ((usage.input_tokens ?? 0) > 0 ||
        (usage.output_tokens ?? 0) > 0 ||
        (usage.cached_input_tokens ?? 0) > 0 ||
        (usage.cached_write_tokens ?? 0) > 0 ||
        (usage.context_tokens ?? 0) > 0),
  )
  const agent = session.runtime_ref?.agent
  const settings = useQuery(agentSettingsQuery)
  const wantsOpenRouter =
    hasTokens &&
    !usage?.context_window_tokens &&
    session.model_provider === 'openrouter' &&
    !!session.model
  const openRouter = useQuery({ ...openRouterModelsQuery, enabled: wantsOpenRouter })

  if (usage?.context_window_tokens) return usage.context_window_tokens
  const known = (agent ? acpAgentModelSuggestions(settings.data, agent) : [])
    .concat(wantsOpenRouter ? (openRouter.data ?? []) : [])
    .find((m) => m.value === session.model)
  return known?.contextLength ?? contextWindowHeuristic(session.model, agent)
}

// Last resort for free-text model ids the catalogs don't know.
function contextWindowHeuristic(model?: string, acpAgent?: string): number | null {
  const id = (model ?? '').toLowerCase()
  if (id.includes('[1m]')) return 1_000_000
  // Claude ACP without an explicit pick runs the adapter default: Opus 4.8 (1M).
  if (acpAgent === 'claude' && id === '') return 1_000_000
  if (acpAgent === 'grok' && id === '') return 512_000
  if (id.startsWith('openrouter/')) return 400_000
  if (id.startsWith('ollama/')) return 128_000
  if (/claude|sonnet|haiku|opus|fable/.test(id)) return 200_000
  if (/gpt-5|codex/.test(id)) return 400_000
  if (/grok-4\.5|grok-build/.test(id)) return 512_000
  if (/grok|composer/.test(id)) return 200_000
  if (/gemini/.test(id)) return 1_000_000
  return null
}
