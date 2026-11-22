import { queryOptions, useQuery } from '@tanstack/react-query'
import { get } from './api/client'
import { isCloudThread } from './api/cloudClient'
import { agentSettingsQuery } from './api/settings'
import { keys } from '@/lib/query/keys'
import type { AgentSettings, ModelCatalogEntry, Session } from './api/types'
import { modelProviderModelsRequest } from './modelProviderRequest'
import { modelSuggestionFor, type ModelSuggestion } from './modelSuggestion'

export {
  filterModelSuggestions,
  modelSuggestionFor,
  modelSuggestionLabel,
  type ModelPricing,
  type ModelSuggestion,
} from './modelSuggestion'

export function acpAgentModelSuggestions(
  settings: AgentSettings | undefined,
  agent: string,
): ModelSuggestion[] {
  return modelSuggestionsFromCatalog(settings?.acp_options?.[agent]?.models ?? [])
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

export function modelCatalogQuery(agent: string, provider?: string) {
  const request = provider === undefined
    ? { queryKey: keys.acpAgentModels(agent), path: `/v1/acp/agents/${encodeURIComponent(agent)}/models` }
    : modelProviderModelsRequest(provider, agent)
  return queryOptions({
    queryKey: request.queryKey,
    queryFn: async () => {
      const body = await get<{ models?: ModelCatalogEntry[]; default_model?: string }>(request.path)
      return { models: modelSuggestionsFromCatalog(body.models ?? []), defaultModel: body.default_model }
    },
    staleTime: provider === undefined ? 60_000 : 60 * 60 * 1000,
    gcTime: Infinity,
    retry: provider === undefined ? false : 5,
    retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 30_000),
  })
}

export const openRouterModelsQuery = modelCatalogQuery('', 'openrouter')

function modelSuggestionsFromCatalog(models: ModelCatalogEntry[]): ModelSuggestion[] {
  return models.map((model) => ({
    value: model.value,
    label: model.label,
    aliases: model.aliases ? [...model.aliases] : undefined,
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
    reasoning: {
      ...model.reasoning,
      efforts: model.reasoning.efforts ? [...model.reasoning.efforts] : undefined,
    },
  }))
}

// Context window for a session, by precedence: the runtime-reported size
// (ACP usage_update), then an explicit provider catalog value. Unknown native
// metadata stays unknown; null hides the capacity readout.
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
  const isCloud = session.backend === 'cloud' || isCloudThread(session.id)
  const agent = session.runtime_ref?.agent
  const settings = useQuery({ ...agentSettingsQuery, enabled: !isCloud })
  const wantsOpenRouter =
    !isCloud &&
    hasTokens &&
    !usage?.context_window_tokens &&
    session.model_provider === 'openrouter' &&
    !!session.model
  const openRouter = useQuery({ ...openRouterModelsQuery, enabled: wantsOpenRouter })

  if (usage?.context_window_tokens) return usage.context_window_tokens
  const known = (agent ? acpAgentModelSuggestions(settings.data, agent) : [])
    .concat(wantsOpenRouter ? (openRouter.data?.models ?? []) : [])
    .find((m) => m.value === session.model)
  return known?.contextLength ?? null
}
