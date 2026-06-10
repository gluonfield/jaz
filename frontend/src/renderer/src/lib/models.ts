import { queryOptions, useQuery } from '@tanstack/react-query'
import type { Session } from './api/types'
import { keys } from './query/keys'

export interface ModelSuggestion {
  value: string
  label: string
  description?: string
  contextLength?: number
}

// Curated per-provider suggestions; free-text model ids are always allowed.
export const OPENAI_MODELS: ModelSuggestion[] = [
  { value: 'gpt-5.5', label: 'GPT-5.5', description: 'Most capable', contextLength: 1_050_000 },
  { value: 'gpt-5.4-mini', label: 'GPT-5.4 Mini', description: 'Fast and inexpensive', contextLength: 400_000 },
  { value: 'gpt-5.3-codex-spark', label: 'GPT-5.3 Codex Spark', description: 'Tuned for coding', contextLength: 400_000 },
]

export const CODEX_ACP_MODELS: ModelSuggestion[] = [
  { value: 'gpt-5.5', label: 'GPT-5.5', description: 'Most capable', contextLength: 1_050_000 },
  { value: 'gpt-5.3-codex-spark', label: 'GPT-5.3 Codex Spark', description: 'Account-gated research preview', contextLength: 400_000 },
  { value: 'gpt-5.4', label: 'GPT-5.4', description: 'Strong coding model', contextLength: 400_000 },
  { value: 'gpt-5.4-mini', label: 'GPT-5.4 Mini', description: 'Fast and inexpensive', contextLength: 400_000 },
]

// Model config values advertised by @agentclientprotocol/claude-agent-acp@0.44.0;
// the backend rejects ids the adapter doesn't advertise.
export const ANTHROPIC_MODELS: ModelSuggestion[] = [
  { value: 'default', label: 'Default (Opus 4.8)', description: 'Opus 4.8 with 1M context · Recommended', contextLength: 1_000_000 },
  { value: 'claude-fable-5[1m]', label: 'Fable 5', description: 'Most capable for the hardest tasks', contextLength: 1_000_000 },
  { value: 'sonnet', label: 'Sonnet 4.6', description: 'Efficient for routine tasks', contextLength: 200_000 },
  { value: 'sonnet[1m]', label: 'Sonnet 4.6 (1M context)', description: 'Draws from usage credits', contextLength: 1_000_000 },
  { value: 'haiku', label: 'Haiku 4.5', description: 'Fastest for quick answers', contextLength: 200_000 },
]

export const GROK_MODELS: ModelSuggestion[] = [
  { value: 'grok-build', label: 'Grok Build', description: 'Best for advanced coding tasks', contextLength: 512_000 },
  { value: 'grok-composer-2.5-fast', label: 'Composer 2.5', description: "Cursor's coding model", contextLength: 200_000 },
]

// ACP agents imply their provider; native resolves through its provider setting.
const ACP_AGENT_MODELS: Record<string, ModelSuggestion[]> = {
  claude: ANTHROPIC_MODELS,
  codex: CODEX_ACP_MODELS,
  grok: GROK_MODELS,
}

export function acpAgentModelSuggestions(agent: string): ModelSuggestion[] {
  return ACP_AGENT_MODELS[agent] ?? []
}

export const openRouterModelsQuery = queryOptions({
  queryKey: keys.openRouterModels,
  queryFn: async (): Promise<ModelSuggestion[]> => {
    const res = await fetch('https://openrouter.ai/api/v1/models?output_modalities=text,image')
    if (!res.ok) throw new Error(`OpenRouter models request failed: ${res.status}`)
    const body = (await res.json()) as {
      data?: { id: string; name?: string; context_length?: number }[]
    }
    return (body.data ?? [])
      .filter((model) => model.id)
      .map((model) => ({
        value: model.id,
        // OpenRouter names lead with the vendor ("OpenAI: GPT-5.4 Mini"); the
        // id in the description carries it, so show just the model name.
        label: (model.name || model.id).replace(/^[^:]+: /, ''),
        description: model.id,
        contextLength: model.context_length || undefined,
      }))
  },
  staleTime: 60 * 60 * 1000,
  retry: 1,
})

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
  const wantsOpenRouter =
    hasTokens &&
    !usage?.context_window_tokens &&
    session.model_provider === 'openrouter' &&
    !!session.model
  const openRouter = useQuery({ ...openRouterModelsQuery, enabled: wantsOpenRouter })

  if (usage?.context_window_tokens) return usage.context_window_tokens
  const known = (agent ? acpAgentModelSuggestions(agent) : [])
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
  if (/claude|sonnet|haiku|opus|fable/.test(id)) return 200_000
  if (/gpt-5|codex/.test(id)) return 400_000
  if (/grok-build/.test(id)) return 512_000
  if (/grok|composer/.test(id)) return 200_000
  if (/gemini/.test(id)) return 1_000_000
  return null
}
