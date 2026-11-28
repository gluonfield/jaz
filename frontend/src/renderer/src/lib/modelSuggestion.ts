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

export function formatModelLabel(value: string, label?: string): string {
  const rawValue = value.trim()
  const rawLabel = label?.trim() ?? ''
  const slug = rawValue.includes('/') ? rawValue.slice(rawValue.indexOf('/') + 1) : rawValue
  if (rawLabel && rawLabel !== rawValue && rawLabel !== slug) {
    return rawLabel
  }
  const source = rawLabel || slug
  if (!source) return ''
  const parts = source.split('-').filter(Boolean)
  if (parts.length <= 1 && !/^(gpt|claude|gemini|grok|kimi)$/i.test(source)) {
    return source
  }
  const tokens: string[] = []
  for (let i = 0; i < parts.length; i += 1) {
    const part = parts[i]
    const lower = part.toLowerCase()
    const next = parts[i + 1]
    if (lower === 'gpt' && next && /^\d+(?:\.\d+)?$/.test(next)) {
      tokens.push(`GPT-${next}`)
      i += 1
      continue
    }
    if (/^\d+$/.test(part) && next && /^\d+$/.test(next) && tokens[0]?.toLowerCase() === 'claude') {
      tokens.push(`${part}.${next}`)
      i += 1
      continue
    }
    if (lower === 'gpt' || lower === 'fp8' || lower === 'fp16' || lower === 'bf16' || /^k\d+$/.test(lower)) {
      tokens.push(part.toUpperCase())
      continue
    }
    if (/^\d+[a-z]+$/i.test(part)) {
      tokens.push(part.toUpperCase())
      continue
    }
    if (/^\d+(?:\.\d+)?$/.test(part)) {
      tokens.push(part)
      continue
    }
    tokens.push(part.slice(0, 1).toUpperCase() + part.slice(1))
  }
  return tokens.join(' ')
}

export function modelSuggestionLabel(suggestions: Pick<ModelSuggestion, 'value' | 'label' | 'aliases'>[], value: string): string {
  const match = modelSuggestionFor(suggestions, value)
  if (match) return match.label
  return value ? formatModelLabel(value) : ''
}
