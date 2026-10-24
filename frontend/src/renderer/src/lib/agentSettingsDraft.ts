import { inputFromSettings } from './agentSettings'
import type { AgentSettings, AgentSettingsInput } from './api/types'

type Draft = { server: AgentSettings; input: AgentSettingsInput }
type Action = { type: 'refresh' | 'reset' | 'edit'; settings: AgentSettings }

export function agentSettingsDraftDirty(draft: Draft | null): boolean {
  return draft != null && JSON.stringify(draft.input) !== JSON.stringify(inputFromSettings(draft.server))
}

export function agentSettingsDraft(draft: Draft | null, action: Action): Draft | null {
  const input = inputFromSettings(action.settings)
  if (action.type === 'edit') return draft ? { ...draft, input } : null
  return {
    server: action.settings,
    input: action.type === 'refresh' && draft && agentSettingsDraftDirty(draft) ? draft.input : input,
  }
}
