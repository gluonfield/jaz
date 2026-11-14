import type { ReasoningEffortOption } from '@/lib/api/types'
import type { ModelSuggestion } from '@/lib/modelSuggestion'

export type ModelPickerOption = Pick<ModelSuggestion, 'value' | 'label' | 'aliases'> & { reasoning?: ModelSuggestion['reasoning'] }

export interface ModelSelection {
  model: string
  effort: string
  fastMode?: boolean
}

export function pickerEffortOptions(options: ReasoningEffortOption[]): ReasoningEffortOption[] {
  const order = ['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra', 'ultracode']
  const ordered = order.flatMap((value) => options.filter((option) => option.value === value))
  const known = new Set(order)
  const custom = options.filter((option) => !known.has(option.value))
  const seen = new Set<string>()
  return [...ordered, ...custom].filter((option) => {
    if (seen.has(option.value)) return false
    seen.add(option.value)
    return true
  })
}

export function parseModelSelections(raw: string | null): Record<string, ModelSelection> {
  try {
    const parsed = JSON.parse(raw ?? '{}')
    return Object.fromEntries(Object.entries(parsed).flatMap(([key, entry]) => {
      const value = entry as Partial<ModelSelection> | null
      return value != null && typeof value.model === 'string' && typeof value.effort === 'string'
        ? [[key, { model: value.model, effort: value.effort, ...(typeof value.fastMode === 'boolean' ? { fastMode: value.fastMode } : {}) }]] : []
    }))
  } catch {
    return {}
  }
}
