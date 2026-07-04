import type { AgentSettings, ReasoningEffortOption } from './api/types'
import type { ModelSuggestion } from './models'

const REASONING_LABELS: Record<string, string> = {
  '': 'Default',
  none: 'None',
  minimal: 'Minimal',
  low: 'Low',
  medium: 'Medium',
  high: 'High',
  xhigh: 'Extra high',
  max: 'Max',
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

export const NO_REASONING_EFFORT_OPTION: ReasoningEffortOption = { value: 'none', label: 'None' }

export function acpReasoningEffortOptions(
  settings: AgentSettings | undefined,
  agent: string,
): ReasoningEffortOption[] {
  const options = settings?.acp_options?.[agent]?.reasoning_efforts
  return options?.length ? options : REASONING_EFFORT_OPTIONS
}

export function reasoningEffortLabel(
  value: string | undefined,
  options: ReasoningEffortOption[] = REASONING_EFFORT_OPTIONS,
): string {
  const effort = value ?? ''
  return options.find((option) => option.value === effort)?.label ?? REASONING_LABELS[effort] ?? (effort || 'Default')
}

// Settings screens treat '' as "no effort configured" (shown as "None") rather
// than "inherit the default".
export function settingsReasoningOptions(
  options: ReasoningEffortOption[] = REASONING_EFFORT_OPTIONS,
): ReasoningEffortOption[] {
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
  const agentOptions = acpReasoningEffortOptions(settings, agent)
  const values = modelReasoningEfforts(model, suggestions)
  if (values === undefined) return agentOptions
  if (values.length === 0) return [NO_REASONING_EFFORT_OPTION]
  return dedupeReasoningOptions([
    { value: '', label: 'Default' },
    ...harnessSupported(values, agentOptions).map(reasoningOption),
  ])
}

export function modelSettingsReasoningEffortOptions(
  settings: AgentSettings | undefined,
  agent: string,
  model: string,
  suggestions: ModelSuggestion[],
): ReasoningEffortOption[] {
  const agentOptions = acpReasoningEffortOptions(settings, agent)
  const values = modelReasoningEfforts(model, suggestions)
  if (values === undefined) return settingsReasoningOptions(agentOptions)
  return settingsReasoningOptions([
    { value: '', label: 'None' },
    ...harnessSupported(values, agentOptions)
      .filter((value) => value !== 'none')
      .map(reasoningOption),
  ])
}

// A model's efforts gated by what the agent harness accepts: an agent CLI
// rejects levels it doesn't know even when the model supports them (e.g.
// codex has no max). 'none' always passes — agents map it to "no reasoning".
function harnessSupported(values: string[], agentOptions: ReasoningEffortOption[]): string[] {
  const supported = new Set(agentOptions.map((option) => option.value))
  return values.filter((value) => value === 'none' || supported.has(value))
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

function modelReasoningEfforts(model: string, suggestions: ModelSuggestion[]): string[] | undefined {
  const value = model.trim()
  const suggestion = suggestions.find((item) => item.value === value) ?? (value === '' ? suggestions[0] : undefined)
  return suggestion?.reasoningEfforts
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
