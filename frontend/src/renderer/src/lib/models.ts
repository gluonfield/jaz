import { queryOptions } from '@tanstack/react-query'
import { keys } from './query/keys'

export interface ModelSuggestion {
  value: string
  label: string
  description?: string
}

// Curated per-provider suggestions; free-text model ids are always allowed.
export const OPENAI_MODELS: ModelSuggestion[] = [
  { value: 'gpt-5.5', label: 'GPT-5.5', description: 'Most capable' },
  { value: 'gpt-5.4-mini', label: 'GPT-5.4 Mini', description: 'Fast and inexpensive' },
  { value: 'gpt-5.3-codex-spark', label: 'GPT-5.3 Codex Spark', description: 'Tuned for coding' },
]

// Model config values advertised by @agentclientprotocol/claude-agent-acp@0.44.0;
// the backend rejects ids the adapter doesn't advertise.
export const ANTHROPIC_MODELS: ModelSuggestion[] = [
  { value: 'default', label: 'Default (Opus 4.8)', description: 'Opus 4.8 with 1M context · Recommended' },
  { value: 'claude-fable-5[1m]', label: 'Fable 5', description: 'Most capable for the hardest tasks' },
  { value: 'sonnet', label: 'Sonnet 4.6', description: 'Efficient for routine tasks' },
  { value: 'sonnet[1m]', label: 'Sonnet 4.6 (1M context)', description: 'Draws from usage credits' },
  { value: 'haiku', label: 'Haiku 4.5', description: 'Fastest for quick answers' },
]

// ACP agents imply their provider; native resolves through its provider setting.
const ACP_AGENT_MODELS: Record<string, ModelSuggestion[]> = {
  claude: ANTHROPIC_MODELS,
  codex: OPENAI_MODELS,
}

export function acpAgentModelSuggestions(agent: string): ModelSuggestion[] {
  return ACP_AGENT_MODELS[agent] ?? []
}

export const openRouterModelsQuery = queryOptions({
  queryKey: keys.openRouterModels,
  queryFn: async (): Promise<ModelSuggestion[]> => {
    const res = await fetch('https://openrouter.ai/api/v1/models?output_modalities=text,image')
    if (!res.ok) throw new Error(`OpenRouter models request failed: ${res.status}`)
    const body = (await res.json()) as { data?: { id: string; name?: string }[] }
    return (body.data ?? [])
      .filter((model) => model.id)
      .map((model) => ({ value: model.id, label: model.name || model.id, description: model.id }))
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

// Best-effort context window by model id; null hides the capacity readout.
export function contextWindowFor(model?: string, acpAgent?: string): number | null {
  const id = (model ?? '').toLowerCase()
  if (id.includes('[1m]')) return 1_000_000
  // Claude ACP without an explicit pick runs the adapter default: Opus 4.8 (1M).
  if (acpAgent === 'claude' && (id === '' || id === 'default')) return 1_000_000
  if (/claude|sonnet|haiku|opus|fable/.test(id)) return 200_000
  if (/gpt-5|codex/.test(id)) return 400_000
  if (/gemini/.test(id)) return 1_000_000
  return null
}
