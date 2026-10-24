import type { ModelReasoningCapabilities } from './api/types'

export interface ModelPricing {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
}

export interface ModelSuggestion {
  value: string
  label: string
  aliases?: string[]
  description?: string
  contextLength?: number
  pricing?: ModelPricing
  openRouterId?: string
  reasoning: ModelReasoningCapabilities
}

export function modelSuggestionFor<T extends Pick<ModelSuggestion, 'value' | 'aliases'>>(
  suggestions: T[],
  value: string,
): T | undefined {
  const raw = value.trim()
  return raw
    ? suggestions.find((suggestion) => suggestion.value === raw || suggestion.aliases?.includes(raw))
    : undefined
}

export function filterModelSuggestions(
  suggestions: ModelSuggestion[],
  query: string,
): ModelSuggestion[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return suggestions
  return suggestions.filter((suggestion) =>
    [suggestion.value, suggestion.label, ...(suggestion.aliases ?? [])]
      .some((value) => value.toLowerCase().includes(needle)),
  )
}

export function modelSuggestionLabel(suggestions: Pick<ModelSuggestion, 'value' | 'label' | 'aliases'>[], value: string): string {
  return modelSuggestionFor(suggestions, value)?.label ?? value
}
