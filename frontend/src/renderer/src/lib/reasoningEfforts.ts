import type { AgentSettings, ReasoningEffortOption } from './api/types'
import { modelSuggestionFor, type ModelSuggestion } from './modelSuggestion'

const REASONING_LABELS: Record<string, string> = {
  '': 'Default',
  none: 'None',
  minimal: 'Minimal',
  low: 'Low',
  medium: 'Medium',
  high: 'High',
  xhigh: 'Extra high',
  max: 'Max',
  ultra: 'Ultra',
  ultracode: 'Ultracode',
}

export const REASONING_EFFORT_OPTIONS: ReasoningEffortOption[] = [
  { value: '', label: 'Default' },
  { value: 'minimal', label: 'Minimal' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra high' },
]

const NO_REASONING_EFFORT_OPTION: ReasoningEffortOption = { value: 'none', label: 'None' }

function acpReasoningEffortOptions(
  settings: AgentSettings | undefined,
  agent: string,
): ReasoningEffortOption[] {
  // An explicit empty list means the agent has no reasoning efforts of its own —
  // e.g. Antigravity, whose thinking level is baked into the model name. Honor
  // that; only fall back to a generic list when the agent's options haven't loaded.
  return settings?.acp_options?.[agent]?.reasoning_efforts ?? REASONING_EFFORT_OPTIONS
}

export function reasoningEffortLabel(
  value: string | undefined,
  options: ReasoningEffortOption[],
): string {
  const effort = value ?? ''
  return options.find((option) => option.value === effort)?.label ?? REASONING_LABELS[effort] ?? (effort || 'Default')
}

// Settings screens treat '' as "no effort configured" (shown as "None") rather
// than "inherit the default".
function settingsReasoningOptions(options: ReasoningEffortOption[]): ReasoningEffortOption[] {
  return dedupeReasoningOptions(options)
    .filter((option) => option.value !== 'none')
    .map((option) => (option.value === '' ? { ...option, label: 'None' } : option))
}

export function modelReasoningEffortOptions(
  settings: AgentSettings | undefined,
  agent: string,
  model: string,
  suggestions: ModelSuggestion[],
): ReasoningEffortOption[] {
  return reasoningOptions(settings, agent, model, suggestions, false)
}

export interface ModelReasoningSelection {
  options: ReasoningEffortOption[]
  effectiveEffort: string
  supported: boolean
  status: 'pending' | 'error' | 'ready'
  blocked: boolean
}

export type ModelReasoningCatalog =
  | { status: 'pending' | 'error' }
  | {
      status: 'ready'
      suggestions: ModelSuggestion[]
      unknownModel: 'ready' | 'unavailable'
    }

export function modelReasoningSelection({
  settings,
  agent,
  model,
  requested,
  settingsMode,
  catalog,
}: {
  settings: AgentSettings | undefined
  agent: string
  model: string
  requested: string
  settingsMode: boolean
  catalog: ModelReasoningCatalog
}): ModelReasoningSelection {
  const effort = requested.trim()
  if (catalog.status !== 'ready') {
    return {
      options: [],
      effectiveEffort: effort,
      supported: true,
      status: catalog.status,
      blocked: effort !== '',
    }
  }
  const suggestion = modelSuggestionFor(catalog.suggestions, model)
  const capabilityStatus = suggestion?.reasoning.status ?? catalog.unknownModel
  if (capabilityStatus === 'pending') {
    return { options: [], effectiveEffort: effort, supported: true, status: 'pending', blocked: effort !== '' }
  }
  if (capabilityStatus === 'unavailable') {
    return { options: [], effectiveEffort: '', supported: effort === '', status: 'ready', blocked: false }
  }
  const options = reasoningOptions(settings, agent, model, catalog.suggestions, settingsMode)
  return {
    options,
    effectiveEffort: effectiveReasoningEffort(effort, options),
    supported: supportedReasoningEffort(effort, options),
    status: 'ready',
    blocked: false,
  }
}

export function supportedReasoningEffort(value: string, options: ReasoningEffortOption[]): boolean {
  const effort = value.trim()
  return effort === '' || options.some((option) => option.value === effort)
}

export function effectiveReasoningEffort(
  requested: string,
  options: ReasoningEffortOption[],
): string {
  const effort = requested.trim()
  if (effort === '' || supportedReasoningEffort(effort, options)) return effort
  return options.some((option) => option.value === 'none') ? 'none' : ''
}

export function inheritedReasoningEffortOverride(
  inherited: string,
  options: ReasoningEffortOption[],
): string | null {
  const effort = inherited.trim()
  if (effort === '' || supportedReasoningEffort(effort, options)) return null
  return options.some((option) => option.value === 'none') ? 'none' : null
}

function reasoningOptions(
  settings: AgentSettings | undefined,
  agent: string,
  model: string,
  suggestions: ModelSuggestion[],
  settingsMode: boolean,
): ReasoningEffortOption[] {
  const suggestion = modelSuggestionFor(suggestions, model)
  if (!suggestion) {
    const agentOptions = acpReasoningEffortOptions(settings, agent)
    return settingsMode ? settingsReasoningOptions(agentOptions) : agentOptions
  }
  if (suggestion.reasoning.status !== 'ready') return []
  const values = suggestion.reasoning.efforts ?? []
  if (!settingsMode && values.length === 0) return [NO_REASONING_EFFORT_OPTION]
  const options = [
    { value: '', label: settingsMode ? 'None' : 'Default' },
    ...values.filter((value) => !settingsMode || value !== 'none').map(reasoningOption),
  ]
  return settingsMode ? settingsReasoningOptions(options) : dedupeReasoningOptions(options)
}

function reasoningOption(value: string): ReasoningEffortOption {
  return { value, label: REASONING_LABELS[value] ?? value }
}

function dedupeReasoningOptions(options: ReasoningEffortOption[]): ReasoningEffortOption[] {
  const seen = new Set<string>()
  const out: ReasoningEffortOption[] = []
  for (const option of options) {
    if (seen.has(option.value)) continue
    seen.add(option.value)
    out.push(option)
  }
  return out
}
