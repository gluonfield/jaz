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
  description?: string
  contextLength?: number
  pricing?: ModelPricing
  openRouterId?: string
  reasoning: ModelReasoningCapabilities
}

export function modelSuggestionFor(
  suggestions: ModelSuggestion[],
  value: string,
): ModelSuggestion | undefined {
  const raw = value.trim()
  return raw ? suggestions.find((suggestion) => suggestion.value === raw) : suggestions[0]
}
