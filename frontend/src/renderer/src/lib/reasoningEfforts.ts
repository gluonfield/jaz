import type { AgentSettings, ReasoningEffortOption } from './api/types'

export const REASONING_EFFORT_OPTIONS: ReasoningEffortOption[] = [
  { value: '', label: 'Default' },
  { value: 'minimal', label: 'Minimal' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra high' },
]

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
  return options.find((option) => option.value === effort)?.label ?? (effort || 'Default')
}

// Settings screens treat '' as "no effort configured" (shown as "None") rather
// than "inherit the default".
export function settingsReasoningOptions(
  options: ReasoningEffortOption[] = REASONING_EFFORT_OPTIONS,
): ReasoningEffortOption[] {
  return options.map((option) => (option.value === '' ? { ...option, label: 'None' } : option))
}
